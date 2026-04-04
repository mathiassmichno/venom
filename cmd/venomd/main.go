package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/mathiassmichno/venom/daemon"
	pb "github.com/mathiassmichno/venom/proto/generated/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/urfave/cli/v3"
)

func main() {
	var verbose bool

	// Check environment variable first for verbose logging
	if os.Getenv("VENOMD_VERBOSE") == "true" {
		verbose = true
	}

	// Set up slog handler based on verbose flag
	var handler slog.Handler
	if verbose {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}
	slog.SetDefault(slog.New(handler))

	var wg sync.WaitGroup
	stopVenomdCtx, cancel := context.WithCancel(context.Background())

	stopSignalCh := make(chan os.Signal, 1)
	signal.Notify(stopSignalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-stopSignalCh
		slog.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	go func() {
		slog.Debug("starting pprof server", "addr", "localhost:6060")
		http.ListenAndServe("localhost:6060", nil)
	}()

	pm := daemon.NewProcManager()

	cmd := &cli.Command{
		Name: "venomd",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:      "host",
				Value:     "localhost",
				UsageText: "host to listen on",
			},
		},
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "port",
				Value: 9988,
				Usage: "port to listen on",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "enable debug logging",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Handle verbose flag that may have been set
			if cmd.Bool("verbose") && !verbose {
				verbose = true
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
			}

			addr := fmt.Sprintf("%s:%d", cmd.StringArg("host"), cmd.Int("port"))
			lis, err := net.Listen("tcp", addr)
			if err != nil {
				slog.Error("failed to create listener", "addr", addr, "err", err)
				cli.Exit(fmt.Sprintf("listen: %v\n", err), 2)
			}

			srv := daemon.NewServer(pm)
			grpcServer := grpc.NewServer()
			pb.RegisterVenomDaemonServer(grpcServer, srv)
			reflection.Register(grpcServer)

			wg.Go(func() {
				slog.Info("gRPC server starting", "addr", addr)
				if err := grpcServer.Serve(lis); err != nil {
					slog.Error("gRPC server error", "err", err)
				}
			})

			wg.Go(func() {
				<-stopVenomdCtx.Done()
				slog.Info("shutting down gRPC server")
				grpcServer.GracefulStop()
			})

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}

	<-stopVenomdCtx.Done()

	slog.Info("shutting down process manager")
	pm.Shutdown()
	slog.Info("shutdown complete")

	wg.Wait()
}
