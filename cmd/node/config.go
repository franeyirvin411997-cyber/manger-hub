package main

import (
	"os"
	"strings"
)

type NodeConfig struct {
	NodeID          string
	ManagerAddr     string
	DisplayName     string
	EnrollmentID    string
	EnrollmentToken string
	TokenFile       string
	Hostname        string
}

func normalizeManagerAddr(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "localhost:50051"
	}
	if strings.Contains(value, ":") {
		return value
	}
	return value + ":50051"
}

func loadNodeConfig() NodeConfig {
	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		hostname = "unknown"
	}

	return NodeConfig{
		NodeID:          "node-" + hostname,
		ManagerAddr:     normalizeManagerAddr(os.Getenv("MANAGER_IP")),
		DisplayName:     strings.TrimSpace(os.Getenv("NODE_DISPLAY_NAME")),
		EnrollmentID:    strings.TrimSpace(os.Getenv("NODE_ENROLLMENT_ID")),
		EnrollmentToken: strings.TrimSpace(os.Getenv("NODE_ENROLLMENT_TOKEN")),
		TokenFile:       strings.TrimSpace(getNodeTokenFile()),
		Hostname:        hostname,
	}
}

func getNodeTokenFile() string {
	if value := os.Getenv("NODE_TOKEN_FILE"); strings.TrimSpace(value) != "" {
		return value
	}
	return "/var/lib/mnp-node/token"
}
