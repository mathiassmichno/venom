package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "github.com/mathiassmichno/venom/api"
	"github.com/mathiassmichno/venom/internal/daemon"
	"google.golang.org/grpc"
)

func main() {
	addr := flag.String("listen", ":50051", "listen address")
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(2)
	}

	pm := daemon.NewProcManager()
	srv := daemon.NewServer(pm)
	grpcServer := grpc.NewServer()
	pb.RegisterVenomDaemonServer(grpcServer, srv)

	go func() {
		fmt.Println("venomd listening on", *addr)
		if err := grpcServer.Serve(lis); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("Shutting down...")
	grpcServer.GracefulStop()
	pm.Shutdown()
}
