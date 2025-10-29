package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	pb "github.com/mathiassmichno/venom/api"
	"github.com/mathiassmichno/venom/internal/daemon"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/urfave/cli/v3"
)

func main() {
	var wg sync.WaitGroup
	stopVenomdCtx, cancel := context.WithCancel(context.Background())

	stopSignalCh := make(chan os.Signal, 1)
	signal.Notify(stopSignalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-stopSignalCh
		fmt.Printf(" got: %s\n", sig)
		cancel()
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
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			addr := fmt.Sprintf("%s:%d", cmd.StringArg("host"), cmd.Int("port"))
			lis, err := net.Listen("tcp", addr)
			if err != nil {
				cli.Exit(fmt.Sprintf("listen: %v\n", err), 2)
			}

			srv := daemon.NewServer(pm)
			grpcServer := grpc.NewServer()
			pb.RegisterVenomDaemonServer(grpcServer, srv)
			reflection.Register(grpcServer)

			wg.Go(func() {
				fmt.Println("venomd listening on", addr)
				time.Sleep(2 * time.Second)
				if err := grpcServer.Serve(lis); err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			})

			wg.Go(func() {
				<-stopVenomdCtx.Done()

				fmt.Println("Shutting down...")
				grpcServer.GracefulStop()
			})

			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}

	<-stopVenomdCtx.Done()

	pm.Shutdown()

	wg.Wait()
}
