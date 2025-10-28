package daemon

import (
	"context"
	"time"

	pb "github.com/mathiassmichno/venom/api"
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
	info, err := s.pm.Start(
		req.Id,
		req.Cmd,
		req.Args,
		req.Cwd,
		req.Env,
	)
	if err != nil {
		return &pb.StartProcessResponse{Success: false, Message: err.Error()}, nil
	}
	return &pb.StartProcessResponse{Id: info.ID, Pid: int32(info.Pid), Success: true}, nil
}

func (s *Server) StopProcess(ctx context.Context, req *pb.StopProcessRequest) (*pb.StopProcessResponse, error) {
	err := s.pm.Stop(req.Id, req.Signal)
	if err != nil {
		return &pb.StopProcessResponse{Success: false, Message: err.Error()}, nil
	}
	return &pb.StopProcessResponse{Success: true}, nil
}

func (s *Server) StreamLogs(req *pb.StreamLogsRequest, stream pb.VenomDaemon_StreamLogsServer) error {
	ch := make(chan LogChunk, 32)
	if err := s.pm.SubscribeLogs(req.Id, req.FromStart, ch); err != nil {
		return err
	}
	defer s.pm.UnsubscribeLogs(req.Id, ch)

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case chunk := <-ch:
			entry := &pb.LogEntry{
				Id:     req.Id,
				Ts:     timestamppb.New(chunk.Time),
				Stream: chunk.Stream,
				Data:   chunk.Data,
			}
			if err := stream.Send(entry); err != nil {
				return err
			}
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
