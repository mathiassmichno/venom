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
	"regexp"
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
					for _, p := range resp.Processes {
						fmt.Printf("%s\t%s\n", p.Id, p.String())
					}
					return nil
				},
			},
			{
				Name:  "start",
				Usage: "Start a new process",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "cwd", Usage: "working directory"},
					&cli.StringSliceFlag{Name: "env", Usage: "envvar \"name=value\" (can repeat)"},
					&cli.DurationFlag{Name: "wait-timeout", Usage: "timeout while waiting", Value: 10 * time.Second},
					&cli.BoolFlag{Name: "wait-for-exit", Usage: "wait for the started process to exit"},
					&cli.StringFlag{Name: "wait-for-regex", Usage: "wait for the output of the started process to match regex"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					args := cmd.Args().Tail()
					dir := cmd.String("dir")
					env := cmd.StringSlice("env")
					if name == "" {
						return cli.Exit("name required", 1)
					}
					if cmd.IsSet("wait-for-exit") && cmd.IsSet("wait-for-regex") {
						return cli.Exit("wait-for-exit and wait-for-regex flags are mutually exclusive", 1)
					}

					ctx, cancel := context.WithTimeout(ctx, timeout)
					defer cancel()
					conn, cmdent, err := dial()
					if err != nil {
						return err
					}
					defer conn.Close()

					req := &pb.StartProcessRequest{
						Definition: &pb.ProcessDefinition{Name: name, Args: args, Dir: dir, Env: env},
					}
					if cmd.Bool("wait-for-exit") {
						req.WaitFor = &pb.StartProcessRequest_Exit{Exit: true}
					} else if regexStr := cmd.String("wait-for-regex"); regexStr != "" {
						if _, err := regexp.Compile(regexStr); err != nil {
							cli.Exit(fmt.Sprintf("wait-for-regex is not a valid regex: %s", err.Error()), 1)
						}
						req.WaitFor = &pb.StartProcessRequest_Regex{Regex: regexStr}
					}
					var rsp *pb.StartProcessResponse
					rsp, err = cmdent.StartProcess(ctx, req)
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
					fmt.Println("stopped", id, rsp.Status.String())
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
