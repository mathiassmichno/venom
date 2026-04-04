package daemon

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"time"

	pb "github.com/mathiassmichno/venom/proto/generated/go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server implements the VenomDaemon gRPC service.
// It wraps a ProcManager to handle process lifecycle via gRPC.
type Server struct {
	pb.UnimplementedVenomDaemonServer
	pm *ProcManager
}

// NewServer creates a new Server wrapping the given ProcManager.
func NewServer(pm *ProcManager) *Server {
	return &Server{pm: pm}
}

// NewProcessStatusFromProcInfo converts internal ProcInfo to protobuf ProcessStatus.
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

// NewProcessStatusFromError creates a ProcessStatus from an error.
func NewProcessStatusFromError(err error) *pb.ProcessStatus {
	return &pb.ProcessStatus{State: &pb.ProcessStatus_Error{Error: err.Error()}}
}

// StartProcess handles the gRPC StartProcess RPC.
// It parses the request, starts the process, and returns the result.
func (s *Server) StartProcess(ctx context.Context, req *pb.StartProcessRequest) (*pb.StartProcessResponse, error) {
	slog.Info("gRPC call: StartProcess", "name", req.Definition.Name, "args", req.Definition.Args)

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
		slog.Warn("StartProcess failed", "name", req.Definition.Name, "err", err)
		return &pb.StartProcessResponse{Success: false, Status: NewProcessStatusFromError(err)}, nil
	}
	slog.Info("gRPC call: StartProcess completed", "id", info.ID, "success", true)
	return &pb.StartProcessResponse{Id: info.ID, Success: true, Status: NewProcessStatusFromProcInfo(info)}, nil
}

// StopProcess handles the gRPC StopProcess RPC.
func (s *Server) StopProcess(ctx context.Context, req *pb.StopProcessRequest) (*pb.StopProcessResponse, error) {
	slog.Info("gRPC call: StopProcess", "id", req.Id, "wait", req.Wait)

	info, err := s.pm.Stop(req.Id, req.Wait)
	if err != nil {
		slog.Warn("StopProcess failed", "id", req.Id, "err", err)
		return &pb.StopProcessResponse{Success: false, Status: NewProcessStatusFromError(err)}, nil
	}
	slog.Info("gRPC call: StopProcess completed", "id", req.Id, "success", true)
	return &pb.StopProcessResponse{Success: true, Status: NewProcessStatusFromProcInfo(info)}, nil
}

// SendProcessInput handles the gRPC SendProcessInput RPC.
// Writes input to the process stdin if it was started with WithStdinPipe.
func (s *Server) SendProcessInput(ctx context.Context, req *pb.ProcessInput) (*pb.ProcessInputWritten, error) {
	id := req.GetId()
	slog.Info("gRPC call: SendProcessInput", "id", id)

	s.pm.RLock()
	info := s.pm.Procs[id]
	s.pm.RUnlock()

	if info.Stdin == nil {
		slog.Warn("SendProcessInput: process not started with stdin pipe", "id", id)
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
		slog.Warn("SendProcessInput write failed", "id", id, "err", err)
		rsp.Result = &pb.ProcessInputWritten_Error{Error: err.Error()}
	} else {
		rsp.Result = &pb.ProcessInputWritten_Status{Status: NewProcessStatusFromProcInfo(info)}
	}

	slog.Info("gRPC call: SendProcessInput completed", "id", id, "written", written)
	return &rsp, nil
}

// StreamLogs handles the gRPC StreamLogs RPC (server-side streaming).
// Subscribes to the process log channel and streams entries to the client.
func (s *Server) StreamLogs(req *pb.StreamLogsRequest, stream pb.VenomDaemon_StreamLogsServer) error {
	id := req.GetId()
	slog.Info("gRPC call: StreamLogs", "id", id, "from_start", req.FromStart)

	s.pm.RLock()
	proc, ok := s.pm.Procs[id]
	s.pm.RUnlock()
	if !ok {
		slog.Warn("StreamLogs: process not found", "id", id)
		return status.Errorf(codes.NotFound, "process %q not found", id)
	}

	sub := proc.Subscribe()
	defer proc.Unsubscribe(sub)

	for {
		select {
		case logEntry, ok := <-sub:
			if !ok {
				slog.Info("gRPC call: StreamLogs completed", "id", id, "reason", "channel_closed")
				return nil
			}
			if err := stream.Send(&pb.LogEntry{Line: logEntry.line, Ts: timestamppb.New(logEntry.ts)}); err != nil {
				slog.Warn("StreamLogs: failed to send", "id", id, "err", err)
				return err
			}
		case <-stream.Context().Done():
			slog.Info("gRPC call: StreamLogs completed", "id", id, "reason", "client_disconnected")
			return nil
		}
	}
}

// ListProcesses handles the gRPC ListProcesses RPC.
// Takes a snapshot of all processes and returns their info.
func (s *Server) ListProcesses(ctx context.Context, _ *emptypb.Empty) (*pb.ListProcessesResponse, error) {
	slog.Info("gRPC call: ListProcesses")

	s.pm.RLock()
	procs := make(map[string]*ProcInfo, len(s.pm.Procs))
	for id, info := range s.pm.Procs {
		procs[id] = info
	}
	s.pm.RUnlock()

	var processes []*pb.ProcessInfo
	for id, info := range procs {
		info.RLock()
		processes = append(processes, &pb.ProcessInfo{
			Id: id,
			Definition: &pb.ProcessDefinition{
				Name: info.Cmd.Name,
				Args: info.Cmd.Args,
				Dir:  info.Cmd.Dir,
				Env:  info.Cmd.Env,
			},
			Status: NewProcessStatusFromProcInfo(info),
		})
		info.RUnlock()
	}
	slog.Info("gRPC call: ListProcesses completed", "count", len(processes))
	return &pb.ListProcessesResponse{Processes: processes}, nil
}

// GetMetrics handles the gRPC GetMetrics RPC.
// Returns current system CPU and memory usage.
func (s *Server) GetMetrics(ctx context.Context, _ *emptypb.Empty) (*pb.MetricsResponse, error) {
	slog.Info("gRPC call: GetMetrics")

	m, err := CollectMetrics()
	if err != nil {
		slog.Warn("GetMetrics failed", "err", err)
		return nil, err
	}
	slog.Debug("Metrics collected", "cpu", m.CPU, "mem_percent", m.MemPercent)
	return &pb.MetricsResponse{
		Ts:         timestamppb.New(time.Now()),
		CpuPercent: m.CPU,
		MemPercent: m.MemPercent,
		MemTotal:   m.MemTotal,
		MemUsed:    m.MemUsed,
	}, nil
}
