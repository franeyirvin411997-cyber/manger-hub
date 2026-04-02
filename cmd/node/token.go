package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"

	pb "multi_node_platform/api/pb"
)

type nodeRegistrar interface {
	Register(ctx context.Context, in *pb.RegisterRequest, opts ...grpc.CallOption) (*pb.RegisterResponse, error)
}

func readPersistedNodeToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func persistNodeToken(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(value)+"\n"), 0o600)
}

func ensureNodeToken(ctx context.Context, client nodeRegistrar, cfg NodeConfig) (string, error) {
	persisted, err := readPersistedNodeToken(cfg.TokenFile)
	if err != nil {
		return "", fmt.Errorf("读取节点 token 失败: %w", err)
	}
	if persisted != "" {
		return persisted, nil
	}

	if cfg.EnrollmentID == "" || cfg.EnrollmentToken == "" {
		return "", fmt.Errorf("缺少节点接入凭证")
	}

	registerRes, err := client.Register(ctx, &pb.RegisterRequest{
		NodeId:          cfg.NodeID,
		Hostname:        cfg.Hostname,
		DisplayName:     cfg.DisplayName,
		Version:         "v2.0.0",
		Capabilities:    []string{"docker", "tun2socks", "tun2proxy", "browser"},
		EnrollmentId:    cfg.EnrollmentID,
		EnrollmentToken: cfg.EnrollmentToken,
	})
	if err != nil {
		return "", err
	}
	if !registerRes.Success {
		return "", fmt.Errorf("注册被拒: %s", registerRes.Message)
	}
	if err := persistNodeToken(cfg.TokenFile, registerRes.Token); err != nil {
		return "", fmt.Errorf("保存节点 token 失败: %w", err)
	}
	return registerRes.Token, nil
}
