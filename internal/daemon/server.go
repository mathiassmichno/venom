package daemon

import (
	"context"
	"fmt"
	"io"
	"regexp"
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

func NewProcessStatusFromProcInfo(procInfo *ProcInfo) *pb.ProcessStatus {
	var ps pb.ProcessStatus
	cs := procInfo.Cmd.Status()
	if cs.Error != nil {
		ps.State = &pb.ProcessStatus_Error{Error: cs.Error.Error()}
	}
	if cs.StartTs > 0 {
		ps.Runtime = durationpb.New(time.Duration(cs.Runtime * float64(time.Second)))
	}
	if cs.StopTs > 0 {
		ps.State = &pb.ProcessStatus_Exit{Exit: uint32(cs.Exit)}
	}
	return &ps
}

func NewProcessStatusFromError(err error) *pb.ProcessStatus {
	return &pb.ProcessStatus{State: &pb.ProcessStatus_Error{Error: err.Error()}}
}

func (s *Server) StartProcess(ctx context.Context, req *pb.StartProcessRequest) (*pb.StartProcessResponse, error) {
	psOpts := ProcStartOptions{
		Dir:           req.Definition.Dir,
		Env:           req.Definition.Env,
		WithStdinPipe: req.WithStdinPipe,
	}

	// handle oneof
	switch waitFor := req.WaitFor.(type) {
	case *pb.StartProcessRequest_Exit:
		psOpts.WaitForExit = waitFor.Exit

	case *pb.StartProcessRequest_Regex:
		re, err := regexp.Compile(waitFor.Regex)
		if err != nil {
			return &pb.StartProcessResponse{
				Success: false,
				Status: &pb.ProcessStatus{
					State: &pb.ProcessStatus_Error{
						Error: fmt.Sprintf("invalid regex: %v", err),
					},
				},
			}, nil
		}
		psOpts.WaitForRegex = re
	}

	// handle optional timeout only if wait_for is set
	if req.WaitFor != nil && req.WaitTimeout != nil {
		d := req.WaitTimeout.AsDuration()
		psOpts.WaitTimeout = &d
	}

	info, err := s.pm.Start(req.Definition.Name, req.Definition.Args, psOpts)
	if err != nil {
		return &pb.StartProcessResponse{Success: false, Status: NewProcessStatusFromError(err)}, nil
	}
	return &pb.StartProcessResponse{Id: info.ID, Success: true, Status: NewProcessStatusFromProcInfo(info)}, nil
}

func (s *Server) StopProcess(ctx context.Context, req *pb.StopProcessRequest) (*pb.StopProcessResponse, error) {
	info, err := s.pm.Stop(req.Id, req.Wait)
	if err != nil {
		return &pb.StopProcessResponse{Success: false, Status: NewProcessStatusFromError(err)}, nil
	}
	return &pb.StopProcessResponse{Success: true, Status: NewProcessStatusFromProcInfo(info)}, nil
}

func (s *Server) SendProcessInput(ctx context.Context, req *pb.ProcessInput) (*pb.ProcessInputWritten, error) {
	id := req.GetId()

	s.pm.RLock()
	info := s.pm.Procs[id]
	s.pm.RUnlock()

	if info.Stdin == nil {
		return nil, fmt.Errorf("process not started with stdin pipe")
	}
	var written int
	var err error
	switch req.Input.(type) {
	case *pb.ProcessInput_Text:
		written, err = io.WriteString(info.Stdin, req.GetText())
	case *pb.ProcessInput_Data:
		written, err = info.Stdin.Write(req.GetData())
	}

	if err == nil && req.Close {
		err = info.Stdin.Close()
	}

	rsp := pb.ProcessInputWritten{Success: err == nil, Written: uint32(written)}
	if err != nil {
		rsp.Result = &pb.ProcessInputWritten_Error{Error: err.Error()}
	} else {
		rsp.Result = &pb.ProcessInputWritten_Status{Status: NewProcessStatusFromProcInfo(info)}
	}

	return &rsp, nil
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
	var processes []*pb.ProcessInfo
	for _, info := range s.pm.Procs {
		processes = append(processes, &pb.ProcessInfo{
			Id: info.ID,
			Definition: &pb.ProcessDefinition{
				Name: info.Cmd.Name,
				Args: info.Cmd.Args,
				Dir:  info.Cmd.Dir,
				Env:  info.Cmd.Env,
			},
			Status: NewProcessStatusFromProcInfo(info),
		})
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
