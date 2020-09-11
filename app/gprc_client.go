package app

import (
	"context"
	"time"

	"github.com/projectrekor/rekor-service/logging"
	"google.golang.org/grpc"
)

func dial(logRpcServer string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set up and test connection to rpc server
	conn, err := grpc.DialContext(ctx, logRpcServer, grpc.WithInsecure())
	if err != nil {
		logging.Logger.Fatalf("Failed to connect to log server:", err)
	}
	return conn, nil
}
