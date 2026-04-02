package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	pb "multi_node_platform/api/pb"
)

type fakeRegisterClient struct {
	registerCalls int
	response      *pb.RegisterResponse
	err           error
}

func (f *fakeRegisterClient) Register(_ context.Context, _ *pb.RegisterRequest, _ ...grpc.CallOption) (*pb.RegisterResponse, error) {
	f.registerCalls++
	return f.response, f.err
}

func TestEnsureNodeTokenUsesPersistedTokenBeforeEnrollment(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "node.token")
	if err := os.WriteFile(tokenFile, []byte("persisted-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	cfg := NodeConfig{
		NodeID:          "node-host",
		Hostname:        "host",
		ManagerAddr:     "manager:50051",
		DisplayName:     "node",
		EnrollmentID:    "enroll-id",
		EnrollmentToken: "enroll-token",
		TokenFile:       tokenFile,
	}
	client := &fakeRegisterClient{
		response: &pb.RegisterResponse{Success: true, Token: "new-token"},
	}

	got, err := ensureNodeToken(context.Background(), client, cfg)
	if err != nil {
		t.Fatalf("ensureNodeToken error: %v", err)
	}
	if got != "persisted-token" {
		t.Fatalf("token = %s", got)
	}
	if client.registerCalls != 0 {
		t.Fatalf("expected no register call, got %d", client.registerCalls)
	}
}

func TestEnsureNodeTokenRegistersAndPersistsWhenTokenMissing(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "node.token")
	cfg := NodeConfig{
		NodeID:          "node-host",
		Hostname:        "host",
		ManagerAddr:     "manager:50051",
		DisplayName:     "node",
		EnrollmentID:    "enroll-id",
		EnrollmentToken: "enroll-token",
		TokenFile:       tokenFile,
	}
	client := &fakeRegisterClient{
		response: &pb.RegisterResponse{Success: true, Token: "registered-token"},
	}

	got, err := ensureNodeToken(context.Background(), client, cfg)
	if err != nil {
		t.Fatalf("ensureNodeToken error: %v", err)
	}
	if got != "registered-token" {
		t.Fatalf("token = %s", got)
	}
	if client.registerCalls != 1 {
		t.Fatalf("expected 1 register call, got %d", client.registerCalls)
	}

	data, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if string(data) != "registered-token\n" {
		t.Fatalf("token file = %q", string(data))
	}
}
