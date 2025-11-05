package daemon

import (
	"context"
	"time"

	pb "github.com/mathiassmichno/venom/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	pb.UnimplementedVenomDaemonServer
	pm *ProcManager
	// TODO: SessionManager
}

func NewServer(pm *ProcManager) *Server {
	return &Server{pm: pm}
}

func (s *Server) StartProcess(ctx context.Context, req *pb.StartProcessRequest) (*pb.StartProcessResponse, error) {
	info, err := s.pm.Start(req.Name, req.Args, ProcStartOptions{
		Dir: req.Dir,
		Env: req.Env,
	})
	if err != nil {
		return &pb.StartProcessResponse{Success: false, Message: err.Error()}, nil
	}
	return &pb.StartProcessResponse{Id: info.ID, Success: true}, nil
}

func (s *Server) StopProcess(ctx context.Context, req *pb.StopProcessRequest) (*pb.StopProcessResponse, error) {
	status, err := s.pm.Stop(req.Id, req.Wait)
	if err != nil {
		return &pb.StopProcessResponse{Success: false, Message: err.Error()}, nil
	}
	if status == nil {
		return &pb.StopProcessResponse{Success: false, Message: "process status was nil!"}, nil
	} else if status.Error != nil {
		return &pb.StopProcessResponse{Success: false, Message: status.Error.Error(), Runtime: durationpb.New(time.Duration(status.Runtime * float64(time.Second)))}, nil
	} else {
		return &pb.StopProcessResponse{Success: true, Exit: uint32(status.Exit), Runtime: durationpb.New(time.Duration(status.Runtime * float64(time.Second)))}, nil
	}
}

func (s *Server) StreamLogs(req *pb.StreamLogsRequest, stream pb.VenomDaemon_StreamLogsServer) error {
	id := req.GetId()

	s.pm.RLock()
	proc, ok := s.pm.Procs[id]
	s.pm.RUnlock()
	if !ok {
		return status.Errorf(codes.NotFound, "process %q not found", id)
	}

	sub := proc.Subscribe()
	defer proc.Unsubscribe(sub)

	for {
		select {
		case logEntry, ok := <-sub:
			if !ok {
				return nil
			}
			if err := stream.Send(&pb.LogEntry{Line: logEntry.line, Ts: timestamppb.New(logEntry.ts)}); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

func (s *Server) ListProcesses(ctx context.Context, _ *emptypb.Empty) (*pb.ListProcessesResponse, error) {
	processes := make(map[string]*pb.ProcessInfo)
	for id, proc := range s.pm.Procs {
		processes[id] = &pb.ProcessInfo{
			Name:    proc.Cmd.Name,
			Args:    proc.Cmd.Args,
			Dir:     proc.Cmd.Dir,
			Env:     proc.Cmd.Env,
			Exit:    uint32(proc.Cmd.Status().Exit),
			Runtime: durationpb.New(time.Duration(proc.Cmd.Status().Runtime * float64(time.Second))),
		}
	}
	return &pb.ListProcessesResponse{Processes: processes}, nil
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
