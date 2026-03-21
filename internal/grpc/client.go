// Package grpc provides a thin client for the TEP plant gRPC API.
// Only exposes what the supervisory reconciler needs:
// - GetPlantStatus (read XMEAS, alarms, ISD)
// - ListControllers (discover existing controllers)
// - UpdateController (adjust parameters of an existing controller)
package grpc

import (
	"context"
	"fmt"
	"time"

	pb "github.com/Green-Cinnamon-Labs/cluster-api-provider-plc/internal/grpc/gen/tepv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// PlantClient wraps the gRPC connection to the TEP plant service.
type PlantClient struct {
	conn   *grpc.ClientConn
	client pb.PlantServiceClient
}

// Connect dials the plant gRPC endpoint with a timeout.
func Connect(ctx context.Context, address string) (*PlantClient, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial plant at %s: %w", address, err)
	}

	return &PlantClient{
		conn:   conn,
		client: pb.NewPlantServiceClient(conn),
	}, nil
}

// GetPlantStatus reads the current plant state: XMEAS, alarms, ISD, controllers.
func (c *PlantClient) GetPlantStatus(ctx context.Context) (*pb.PlantStatus, error) {
	return c.client.GetPlantStatus(ctx, &pb.GetPlantStatusRequest{})
}

// ListControllers returns the controllers currently running on the plant.
func (c *PlantClient) ListControllers(ctx context.Context) ([]*pb.ControllerInfo, error) {
	resp, err := c.client.ListControllers(ctx, &pb.ListControllersRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Controllers, nil
}

// UpdateController adjusts parameters of an existing controller.
func (c *PlantClient) UpdateController(ctx context.Context, req *pb.UpdateControllerRequest) (*pb.UpdateControllerResponse, error) {
	return c.client.UpdateController(ctx, req)
}

// Close releases the gRPC connection.
func (c *PlantClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
