package app

import (
	"context"
	"time"

	"github.com/projectrekor/rekor-server/logging"
	"google.golang.org/grpc"
)

func dial(ctx context.Context, logRpcServer string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Set up and test connection to rpc server
	conn, err := grpc.DialContext(ctx, logRpcServer, grpc.WithInsecure())
	if err != nil {
		logging.Logger.Fatalf("Failed to connect to log server:", err)
	}
	return conn, nil
}
