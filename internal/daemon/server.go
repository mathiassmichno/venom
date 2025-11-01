package daemon

import (
	"context"
	"time"

	pb "github.com/mathiassmichno/venom/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	pb.UnimplementedVenomDaemonServer
	pm *ProcManager
}

func NewServer(pm *ProcManager) *Server {
	return &Server{pm: pm}
}

func (s *Server) StartProcess(ctx context.Context, req *pb.StartProcessRequest) (*pb.StartProcessResponse, error) {
	info, err := s.pm.Start(ProcStartOptions{
		ID:   req.Id,
		Cmd:  req.Cmd,
		Args: req.Args,
		Cwd:  req.Cwd,
		Env:  req.Env,
	})
	if err != nil {
		return &pb.StartProcessResponse{Success: false, Message: err.Error()}, nil
	}
	return &pb.StartProcessResponse{Id: info.ID, Pid: int32(info.Pid), Success: true}, nil
}

func (s *Server) StopProcess(ctx context.Context, req *pb.StopProcessRequest) (*pb.StopProcessResponse, error) {
	_, err := s.pm.Stop(req.Id, req.Signal, false)
	if err != nil {
		return &pb.StopProcessResponse{Success: false, Message: err.Error()}, nil
	}
	return &pb.StopProcessResponse{Success: true}, nil
}

func (s *Server) StreamLogs(req *pb.StreamLogsRequest, stream pb.VenomDaemon_StreamLogsServer) error {
	id := req.GetId()

	s.pm.mu.RLock()
	proc, ok := s.pm.procs[id]
	s.pm.mu.RUnlock()
	if !ok {
		return status.Errorf(codes.NotFound, "process %q not found", id)
	}

	sub := proc.Logs.Subscribe()
	defer proc.Logs.Unsubscribe(sub)

	for {
		select {
		case line, ok := <-sub:
			if !ok {
				return nil
			}
			logEntry := &pb.LogEntry{
				Id: id,
				// TODO: back to timed lines....
				Ts:   timestamppb.New(time.Now()),
				Line: line,
			}
			if err := stream.Send(logEntry); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

func (s *Server) GetMetrics(ctx context.Context, _ *emptypb.Empty) (*pb.MetricsResponse, error) {
	m, err := CollectMetrics()
	if err != nil {
		return nil, err
	}
	return &pb.MetricsResponse{
		Ts:         timestamppb.New(time.Now()),
		CpuPercent: m.CPU,
		MemPercent: m.MemPercent,
		MemTotal:   m.MemTotal,
		MemUsed:    m.MemUsed,
	}, nil
}
