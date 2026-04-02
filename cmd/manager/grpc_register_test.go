package main

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc/peer"

	pb "multi_node_platform/api/pb"
	"multi_node_platform/pkg/models"
)

func TestRegisterRequiresEnrollment(t *testing.T) {
	setupManagerTestRouter(t)
	server := &GrpcServer{}
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.20"), Port: 50051},
	})

	res, err := server.Register(ctx, &pb.RegisterRequest{
		NodeId:       "node-host-a",
		Hostname:     "host-a",
		Version:      "v2.0.0",
		Capabilities: []string{"docker"},
	})
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected anonymous register to fail: %+v", res)
	}
}

func TestRegisterConsumesValidEnrollment(t *testing.T) {
	setupManagerTestRouter(t)
	server := &GrpcServer{}
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.21"), Port: 50051},
	})

	enrollment := models.NodeEnrollment{
		ID:              "enroll-1",
		DisplayName:     "tokyo-01",
		ManagerHTTPAddr: "https://manager.example.com",
		ManagerGrpcAddr: "manager.example.com:50051",
		TokenHash:       hashEnrollmentToken("secret-token"),
		ExpiresAt:       time.Now().UTC().Add(24 * time.Hour),
	}
	if err := models.DB.Create(&enrollment).Error; err != nil {
		t.Fatalf("seed enrollment: %v", err)
	}

	res, err := server.Register(ctx, &pb.RegisterRequest{
		NodeId:          "node-host-b",
		Hostname:        "host-b",
		DisplayName:     "",
		Version:         "v2.0.0",
		Capabilities:    []string{"docker", "browser"},
		EnrollmentId:    "enroll-1",
		EnrollmentToken: "secret-token",
	})
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if !res.Success || res.Token == "" {
		t.Fatalf("expected successful register, got %+v", res)
	}

	var node models.Node
	if err := models.DB.Where("id = ?", "node-host-b").First(&node).Error; err != nil {
		t.Fatalf("load node: %v", err)
	}
	if node.DisplayName != "tokyo-01" {
		t.Fatalf("display name = %s", node.DisplayName)
	}
	if node.IP != "203.0.113.21" {
		t.Fatalf("node IP = %s", node.IP)
	}

	var used models.NodeEnrollment
	if err := models.DB.Where("id = ?", "enroll-1").First(&used).Error; err != nil {
		t.Fatalf("reload enrollment: %v", err)
	}
	if used.UsedAt == nil {
		t.Fatal("expected enrollment to be consumed")
	}
	if used.UsedByNodeID != "node-host-b" {
		t.Fatalf("used_by_node_id = %s", used.UsedByNodeID)
	}
}
