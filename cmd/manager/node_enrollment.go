package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"multi_node_platform/pkg/models"
)

func generateEnrollmentToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashEnrollmentToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func nodeEnrollmentStatus(enrollment models.NodeEnrollment, now time.Time) string {
	if enrollment.UsedAt != nil {
		return "used"
	}
	if now.After(enrollment.ExpiresAt) {
		return "expired"
	}
	return "pending"
}

func buildInstallCommand(baseHTTPAddr, enrollmentID, token string) string {
	return fmt.Sprintf(
		"curl -fsSL '%s/api/v1/node-enrollments/%s/install.sh?token=%s' | bash",
		strings.TrimRight(baseHTTPAddr, "/"),
		enrollmentID,
		token,
	)
}

func validateEnrollmentCreateInput(httpAddr, grpcAddr string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(httpAddr))
	if err != nil {
		return fmt.Errorf("manager_http_addr 无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("manager_http_addr 仅支持 http/https")
	}
	if strings.TrimSpace(grpcAddr) == "" {
		return fmt.Errorf("manager_grpc_addr 不能为空")
	}
	return nil
}

func listNodeEnrollments() ([]models.NodeEnrollment, error) {
	var enrollments []models.NodeEnrollment
	if err := models.DB.Order("created_at desc").Find(&enrollments).Error; err != nil {
		return nil, err
	}
	sort.SliceStable(enrollments, func(i, j int) bool {
		return enrollments[i].CreatedAt.After(enrollments[j].CreatedAt)
	})
	return enrollments, nil
}

func newNodeEnrollment(displayName, httpAddr, grpcAddr string) (models.NodeEnrollment, string, error) {
	token, err := generateEnrollmentToken()
	if err != nil {
		return models.NodeEnrollment{}, "", err
	}

	enrollment := models.NodeEnrollment{
		ID:              uuid.New().String(),
		DisplayName:     strings.TrimSpace(displayName),
		ManagerHTTPAddr: strings.TrimRight(strings.TrimSpace(httpAddr), "/"),
		ManagerGrpcAddr: strings.TrimSpace(grpcAddr),
		TokenHash:       hashEnrollmentToken(token),
		ExpiresAt:       time.Now().UTC().Add(24 * time.Hour),
	}
	return enrollment, token, nil
}

func findUsableEnrollment(id, token string) (*models.NodeEnrollment, error) {
	var enrollment models.NodeEnrollment
	if err := models.DB.Where("id = ?", id).First(&enrollment).Error; err != nil {
		return nil, fmt.Errorf("接入授权不存在")
	}
	if enrollment.UsedAt != nil {
		return nil, fmt.Errorf("接入授权已被使用")
	}
	if time.Now().UTC().After(enrollment.ExpiresAt) {
		return nil, fmt.Errorf("接入授权已过期")
	}
	if enrollment.TokenHash != hashEnrollmentToken(token) {
		return nil, fmt.Errorf("接入授权无效")
	}
	return &enrollment, nil
}

func shellQuoteEnv(value string) string {
	return strings.ReplaceAll(value, "'", `'\''`)
}

func renderNodeInstallScript(enrollment *models.NodeEnrollment, plainToken string) string {
	managerHTTPAddr := shellQuoteEnv(enrollment.ManagerHTTPAddr)
	managerGrpcAddr := shellQuoteEnv(enrollment.ManagerGrpcAddr)
	displayName := shellQuoteEnv(enrollment.DisplayName)
	token := shellQuoteEnv(plainToken)

	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 或 sudo 执行此脚本" >&2
  exit 1
fi

ARCH="$(uname -m)"
if [ "$ARCH" != "x86_64" ] && [ "$ARCH" != "amd64" ]; then
  echo "当前仅支持 linux/amd64 节点安装" >&2
  exit 1
fi

install_packages() {
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    apt-get install -y curl tar ca-certificates docker.io
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y curl tar ca-certificates docker
  elif command -v yum >/dev/null 2>&1; then
    yum install -y curl tar ca-certificates docker
  else
    echo "当前系统暂不支持自动安装" >&2
    exit 1
  fi
}

install_packages
systemctl enable --now docker
mkdir -p /opt/mnp /var/lib/mnp-node
curl -fsSL "%s/download/node" -o /opt/mnp/node
chmod +x /opt/mnp/node

cat >/etc/mnp-node.env <<'EOF'
MANAGER_IP=%s
NODE_DISPLAY_NAME=%s
NODE_ENROLLMENT_ID=%s
NODE_ENROLLMENT_TOKEN=%s
NODE_TOKEN_FILE=/var/lib/mnp-node/token
EOF

cat >/etc/systemd/system/mnp-node.service <<'EOF'
[Unit]
Description=MNP Node
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/mnp-node.env
ExecStart=/opt/mnp/node
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now mnp-node
echo "安装完成，可使用以下命令查看状态："
echo "systemctl status mnp-node --no-pager"
echo "journalctl -u mnp-node -f"
`, managerHTTPAddr, managerGrpcAddr, displayName, enrollment.ID, token)
}

func consumeEnrollment(enrollment *models.NodeEnrollment, nodeID string) error {
	now := time.Now().UTC()
	return models.DB.Model(enrollment).Updates(map[string]interface{}{
		"used_at":         &now,
		"used_by_node_id": nodeID,
	}).Error
}
