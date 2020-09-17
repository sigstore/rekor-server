package app

import (
	"context"
	"time"

	"github.com/projectrekor/rekor-server/logging"
	"google.golang.org/grpc"
)

func dial(ctx context.Context, rpcServer string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Set up and test connection to rpc server
	conn, err := grpc.DialContext(ctx, rpcServer, grpc.WithInsecure())
	if err != nil {
		logging.Logger.Fatalf("Failed to connect to RPC server:", err)
	}
	return conn, nil
}
