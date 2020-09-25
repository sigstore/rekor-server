package app

import (
	"context"
	"time"

	"github.com/projectrekor/rekor-server/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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

func getGprcCode(state codes.Code) string {
	var codeResponse string
	switch state {
	case codes.OK:
		codeResponse = "OK"
	case codes.NotFound:
		codeResponse = "Leaf not Found"
	case codes.AlreadyExists:
		codeResponse = "Data Already Exists"
	default:
		codeResponse = "Error. Unknown Code!"
	}
	return codeResponse
}
