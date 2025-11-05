// venomctl - a small CLI to talk to venomd via gRPC
// Single-file starter using urfave/cli/v3. Adapt the pb import path to your project.

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/mathiassmichno/venom/api"
)

var (
	addr    string
	timeout time.Duration
)

func dial() (*grpc.ClientConn, pb.VenomDaemonClient, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, nil, err
	}
	client := pb.NewVenomDaemonClient(conn)
	return conn, client, nil
}

func main() {
	app := &cli.Command{
		Name:  "venomctl",
		Usage: "Control venomd (start/stop/list/logs/send-input)",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "addr", Value: "localhost:9988", Usage: "daemon gRPC address (unix:// or host:port)", Destination: &addr},
			// &cli.StringFlag{Name: "addr", Value: "unix:///var/run/venomd.sock", Usage: "daemon gRPC address (unix:// or host:port)", Destination: &addr},
			&cli.DurationFlag{Name: "timeout", Value: 10 * time.Second, Usage: "RPC timeout", Destination: &timeout},
		},
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List running processes",
				Action: func(ctx context.Context, cli *cli.Command) error {
					ctx, cancel := context.WithTimeout(context.Background(), timeout)
					defer cancel()
					conn, client, err := dial()
					if err != nil {
						return err
					}
					defer conn.Close()

					resp, err := client.ListProcesses(ctx, &emptypb.Empty{})
					if err != nil {
						return err
					}
					for id, p := range resp.Processes {
						fmt.Printf("%s\t%s\n", id, p.String())
					}
					return nil
				},
			},
			{
				Name:  "start",
				Usage: "Start a new process",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "arg", Usage: "argument (can repeat)"},
					&cli.StringFlag{Name: "cwd", Usage: "working directory"},
				},
				Action: func(ctx context.Context, cli *cli.Command) error {
					name := cli.Args().First()
					argv := cli.Args().Tail()
					dir := cli.String("dir")
					fmt.Println(name, argv)
					if name == "" {
						return fmt.Errorf("name required")
					}

					ctx, cancel := context.WithTimeout(ctx, timeout)
					defer cancel()
					conn, client, err := dial()
					if err != nil {
						return err
					}
					defer conn.Close()

					req := &pb.StartProcessRequest{Name: name, Args: argv, Dir: dir}
					var rsp *pb.StartProcessResponse
					rsp, err = client.StartProcess(ctx, req)
					if err != nil {
						return err
					}
					fmt.Println("started", rsp.Id)
					return nil
				},
			},
			{
				Name:      "stop",
				Usage:     "Stop a running process",
				ArgsUsage: "<id>",
				Action: func(ctx context.Context, cli *cli.Command) error {
					if cli.NArg() != 1 {
						return fmt.Errorf("id required")
					}
					id := cli.Args().Get(0)
					ctx, cancel := context.WithTimeout(ctx, timeout)
					defer cancel()
					conn, client, err := dial()
					if err != nil {
						return err
					}
					defer conn.Close()

					var rsp *pb.StopProcessResponse
					rsp, err = client.StopProcess(ctx, &pb.StopProcessRequest{Id: id})
					if err != nil {
						return err
					}
					fmt.Println("stopped", id, "exit code", rsp.Exit, rsp.Message)
					return nil
				},
			},
			{
				Name:      "logs",
				Usage:     "Stream logs for a process (stdout/stderr)",
				ArgsUsage: "<id>",
				Action: func(ctx context.Context, cli *cli.Command) error {
					if cli.NArg() != 1 {
						return fmt.Errorf("id required")
					}
					id := cli.Args().First()
					conn, client, err := dial()
					if err != nil {
						return err
					}
					defer conn.Close()

					stream, err := client.StreamLogs(ctx, &pb.StreamLogsRequest{Id: id, FromStart: true})
					if err != nil {
						return err
					}

					// handle Ctrl-C locally to cancel streaming
					sigc := make(chan os.Signal, 1)
					signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
					go func() {
						<-sigc
						log.Println("interrupt, stopping log stream")
						// best-effort: cancel context by exiting process
						os.Exit(0)
					}()

					for {
						le, err := stream.Recv()
						if err == io.EOF {
							break
						}
						if err != nil {
							return err
						}
						streamName := "out"
						if le.Stream == pb.LogEntry_STDERR {
							streamName = "err"
						}
						fmt.Printf("[%s] %s\n", streamName, le.Line)
					}
					return nil
				},
			},
			{
				Name:  "version",
				Usage: "Show client version",
				Action: func(ctx context.Context, cli *cli.Command) error {
					fmt.Println("venomctl (unversioned)")
					return nil
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
