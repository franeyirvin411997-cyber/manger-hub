package main

import "testing"

func TestLoadNodeConfigUsesEnrollmentEnv(t *testing.T) {
	t.Setenv("HOSTNAME", "tokyo-host")
	t.Setenv("MANAGER_IP", "manager.example.com:50051")
	t.Setenv("NODE_DISPLAY_NAME", "Tokyo 01")
	t.Setenv("NODE_ENROLLMENT_ID", "enroll-1")
	t.Setenv("NODE_ENROLLMENT_TOKEN", "secret-token")

	cfg := loadNodeConfig()

	if cfg.NodeID != "node-tokyo-host" {
		t.Fatalf("node id = %s", cfg.NodeID)
	}
	if cfg.ManagerAddr != "manager.example.com:50051" {
		t.Fatalf("manager addr = %s", cfg.ManagerAddr)
	}
	if cfg.DisplayName != "Tokyo 01" {
		t.Fatalf("display name = %s", cfg.DisplayName)
	}
	if cfg.EnrollmentID != "enroll-1" {
		t.Fatalf("enrollment id = %s", cfg.EnrollmentID)
	}
	if cfg.EnrollmentToken != "secret-token" {
		t.Fatalf("enrollment token = %s", cfg.EnrollmentToken)
	}
	if cfg.TokenFile != "/var/lib/mnp-node/token" {
		t.Fatalf("token file = %s", cfg.TokenFile)
	}
}

func TestNormalizeManagerAddrAddsDefaultPort(t *testing.T) {
	if got := normalizeManagerAddr("manager.internal"); got != "manager.internal:50051" {
		t.Fatalf("normalized addr = %s", got)
	}
	if got := normalizeManagerAddr("manager.internal:51000"); got != "manager.internal:51000" {
		t.Fatalf("normalized addr = %s", got)
	}
	if got := normalizeManagerAddr(""); got != "localhost:50051" {
		t.Fatalf("normalized addr = %s", got)
	}
}
