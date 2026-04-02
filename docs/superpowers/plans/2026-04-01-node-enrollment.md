# Node Enrollment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add secure one-time node enrollment so the Nodes page can generate a Linux install command, remote VPS hosts can install `node` as a `systemd` service, and only enrolled nodes can register with manager.

**Architecture:** Introduce a manager-owned `NodeEnrollment` persistence model plus HTTP APIs for creating, listing, and rendering install scripts. Enforce enrollment during gRPC `Register`, then teach the node binary to load enrollment settings from environment variables and submit them during first registration. Extend the Vue Nodes page to create enrollments, show the generated install command once, and display enrollment history.

**Tech Stack:** Go 1.25, Gin, GORM, gRPC/protobuf, Vue 3, Element Plus, Playwright, SQLite test driver for Go tests

---

## File Structure

- Create: `cmd/manager/http_nodes.go`
  - Node CRUD handlers moved out of `http_server.go`
  - New node enrollment HTTP handlers
- Create: `cmd/manager/node_enrollment.go`
  - Enrollment token hashing, validation, status derivation, install script rendering
- Create: `cmd/manager/node_enrollment_http_test.go`
  - HTTP tests for create/list/install-script endpoints
- Create: `cmd/manager/grpc_register_test.go`
  - gRPC registration tests for enrollment enforcement
- Create: `cmd/manager/test_helpers_test.go`
  - Shared in-memory DB/router setup for manager tests
- Create: `cmd/node/config.go`
  - Env parsing and manager address normalization for node process
- Create: `cmd/node/config_test.go`
  - Unit tests for node env parsing
- Create: `web/tests/node-enrollment.spec.js`
  - Playwright coverage for the Nodes page enrollment flow
- Modify: `api/proto/platform.proto`
  - Add enrollment and display-name fields to `RegisterRequest`
- Modify: `api/pb/platform.pb.go`
  - Regenerated protobuf types
- Modify: `api/pb/platform_grpc.pb.go`
  - Regenerated protobuf stubs
- Modify: `cmd/manager/grpc_server.go`
  - Reject anonymous registration and consume valid enrollments
- Modify: `cmd/manager/http_server.go`
  - Register the new node enrollment routes and keep only router wiring
- Modify: `pkg/models/models.go`
  - Add `NodeEnrollment` GORM model
- Modify: `pkg/models/db.go`
  - AutoMigrate the new model
- Modify: `go.mod`
  - Add SQLite driver for Go tests
- Modify: `go.sum`
  - Capture the SQLite dependency checksums
- Modify: `cmd/node/main.go`
  - Use parsed config, pass enrollment fields in `Register`, stop appending `:50051` blindly
- Modify: `web/src/views/NodeList.vue`
  - Add toolbar button, enrollment dialog, install-command result view, and enrollment history table
- Modify: `web/package.json`
  - Add a Playwright script for targeted E2E execution

## Task 1: Manager Enrollment HTTP API and Persistence

**Files:**
- Create: `cmd/manager/http_nodes.go`
- Create: `cmd/manager/node_enrollment.go`
- Create: `cmd/manager/node_enrollment_http_test.go`
- Create: `cmd/manager/test_helpers_test.go`
- Modify: `cmd/manager/http_server.go`
- Modify: `pkg/models/models.go`
- Modify: `pkg/models/db.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Write the failing HTTP tests for create/list enrollment**

Create `cmd/manager/test_helpers_test.go` with an isolated SQLite-backed router helper:

```go
package main

import (
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"multi_node_platform/pkg/models"
)

func setupManagerTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	models.DB = db
	if err := models.DB.AutoMigrate(&models.Node{}, &models.NodeEnrollment{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	t.Setenv("ADMIN_USER", "admin")
	t.Setenv("ADMIN_PASSWORD", "admin")
	return SetupGinRouter()
}
```

Create `cmd/manager/node_enrollment_http_test.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateAndListNodeEnrollment(t *testing.T) {
	router := setupManagerTestRouter(t)

	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/node-enrollments",
		strings.NewReader(`{"display_name":"tokyo-01","manager_http_addr":"https://manager.example.com","manager_grpc_addr":"manager.example.com:50051"}`),
	)
	createReq.SetBasicAuth("admin", "admin")
	createReq.Header.Set("Content-Type", "application/json")

	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Data struct {
			ID             string `json:"id"`
			InstallCommand string `json:"install_command"`
			ExpiresAt      string `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.Data.ID == "" {
		t.Fatal("expected enrollment id")
	}
	if !strings.Contains(created.Data.InstallCommand, "/api/v1/node-enrollments/"+created.Data.ID+"/install.sh?token=") {
		t.Fatalf("unexpected install command: %s", created.Data.InstallCommand)
	}
	if created.Data.ExpiresAt == "" {
		t.Fatal("expected expires_at")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/node-enrollments", nil)
	listReq.SetBasicAuth("admin", "admin")
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}

	var listed struct {
		Data []struct {
			DisplayName string `json:"display_name"`
			Status      string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listed.Data) != 1 {
		t.Fatalf("expected 1 enrollment, got %d", len(listed.Data))
	}
	if listed.Data[0].DisplayName != "tokyo-01" {
		t.Fatalf("display_name = %s", listed.Data[0].DisplayName)
	}
	if listed.Data[0].Status != "pending" {
		t.Fatalf("status = %s", listed.Data[0].Status)
	}
}
```

- [ ] **Step 2: Run the manager HTTP tests to verify they fail**

Run:

```bash
go test ./cmd/manager -run TestCreateAndListNodeEnrollment -v
```

Expected: FAIL because `NodeEnrollment` does not exist yet and `/api/v1/node-enrollments` is not registered.

- [ ] **Step 3: Implement the model, helpers, and HTTP handlers**

Add the new GORM model in `pkg/models/models.go`:

```go
type NodeEnrollment struct {
	ID              string         `gorm:"primaryKey;type:varchar(64)" json:"id"`
	DisplayName     string         `gorm:"type:varchar(128)" json:"display_name"`
	ManagerHTTPAddr string         `gorm:"type:varchar(255)" json:"manager_http_addr"`
	ManagerGrpcAddr string         `gorm:"type:varchar(255)" json:"manager_grpc_addr"`
	TokenHash       string         `gorm:"type:char(64)" json:"-"`
	ExpiresAt       time.Time      `json:"expires_at"`
	UsedAt          *time.Time     `json:"used_at"`
	UsedByNodeID    string         `gorm:"type:varchar(64)" json:"used_by_node_id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}
```

Register the model in `pkg/models/db.go`:

```go
	err = DB.AutoMigrate(
		&Node{},
		&NodeEnrollment{},
		&ProxyResource{},
		&ProxyLease{},
		&GroupSpec{},
		&GroupRuntime{},
		&AppTemplate{},
		&AppAccount{},
		&AccountGroupBinding{},
		&BrowserInstance{},
		&Operation{},
		&Task{},
		&Rule{},
		&SystemConfig{},
		&AIActionLog{},
	)
```

Add shared enrollment helpers in `cmd/manager/node_enrollment.go`:

```go
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
		"curl -fsSL '%s/api/v1/node-enrollments/%s/install.sh?token=%s' | sudo bash",
		strings.TrimRight(baseHTTPAddr, "/"),
		enrollmentID,
		token,
	)
}

func validateEnrollmentCreateInput(httpAddr, grpcAddr string) error {
	parsed, err := url.ParseRequestURI(httpAddr)
	if err != nil {
		return fmt.Errorf("manager_http_addr 无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("manager_http_addr 仅支持 http/https")
	}
	if grpcAddr == "" {
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
```

Implement node and enrollment handlers in `cmd/manager/http_nodes.go`:

```go
package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"multi_node_platform/pkg/models"
)

func getNodes(c *gin.Context) {
	var nodes []models.Node
	models.DB.Find(&nodes)
	c.JSON(http.StatusOK, gin.H{"data": nodes})
}

func updateNode(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		DisplayName *string `json:"display_name"`
		MaxGroups   *int    `json:"max_groups"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates := map[string]interface{}{}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.MaxGroups != nil {
		updates["max_groups"] = *req.MaxGroups
	}
	models.DB.Model(&models.Node{}).Where("id = ?", id).Updates(updates)
	c.JSON(http.StatusOK, gin.H{"message": "节点已更新"})
}

func deleteNode(c *gin.Context) {
	id := c.Param("id")
	var count int64
	models.DB.Model(&models.GroupSpec{}).Where("node_id = ?", id).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "该节点上还有代理组，请先迁移或删除"})
		return
	}
	models.DB.Where("id = ?", id).Delete(&models.Node{})
	c.JSON(http.StatusOK, gin.H{"message": "节点已删除"})
}

func createNodeEnrollment(c *gin.Context) {
	var req struct {
		DisplayName     string `json:"display_name"`
		ManagerHTTPAddr string `json:"manager_http_addr" binding:"required"`
		ManagerGrpcAddr string `json:"manager_grpc_addr" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateEnrollmentCreateInput(req.ManagerHTTPAddr, req.ManagerGrpcAddr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	enrollment, token, err := newNodeEnrollment(req.DisplayName, req.ManagerHTTPAddr, req.ManagerGrpcAddr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建接入授权失败"})
		return
	}
	if err := models.DB.Create(&enrollment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存接入授权失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"id":              enrollment.ID,
		"expires_at":      enrollment.ExpiresAt,
		"install_command": buildInstallCommand(enrollment.ManagerHTTPAddr, enrollment.ID, token),
	}})
}

func listNodeEnrollmentsHandler(c *gin.Context) {
	enrollments, err := listNodeEnrollments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询接入授权失败"})
		return
	}

	now := time.Now().UTC()
	data := make([]gin.H, 0, len(enrollments))
	for _, enrollment := range enrollments {
		data = append(data, gin.H{
			"id":               enrollment.ID,
			"display_name":     enrollment.DisplayName,
			"manager_http_addr": enrollment.ManagerHTTPAddr,
			"manager_grpc_addr": enrollment.ManagerGrpcAddr,
			"expires_at":       enrollment.ExpiresAt,
			"used_at":          enrollment.UsedAt,
			"used_by_node_id":  enrollment.UsedByNodeID,
			"created_at":       enrollment.CreatedAt,
			"status":           nodeEnrollmentStatus(enrollment, now),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}
```

Wire the routes in `cmd/manager/http_server.go`:

```go
	// 节点
	api.GET("/nodes", getNodes)
	api.PUT("/nodes/:id", updateNode)
	api.DELETE("/nodes/:id", deleteNode)
	api.GET("/node-enrollments", listNodeEnrollmentsHandler)
	api.POST("/node-enrollments", createNodeEnrollment)
```

Add the SQLite test dependency in `go.mod`:

```go
require (
	github.com/Knetic/govaluate v3.0.0+incompatible
	github.com/gin-gonic/gin v1.12.0
	github.com/google/uuid v1.6.0
	google.golang.org/grpc v1.64.0
	google.golang.org/protobuf v1.36.10
	gorm.io/driver/postgres v1.6.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)
```

- [ ] **Step 4: Run the manager HTTP tests to verify they pass**

Run:

```bash
go test ./cmd/manager -run TestCreateAndListNodeEnrollment -v
```

Expected: PASS with one green test covering create/list behavior.

- [ ] **Step 5: Commit the HTTP enrollment foundation**

```bash
git add cmd/manager/http_nodes.go cmd/manager/node_enrollment.go cmd/manager/node_enrollment_http_test.go cmd/manager/test_helpers_test.go cmd/manager/http_server.go pkg/models/models.go pkg/models/db.go go.mod go.sum
git commit -m "feat: add node enrollment HTTP API"
```

## Task 2: Install Script Rendering and Validation

**Files:**
- Modify: `cmd/manager/http_nodes.go`
- Modify: `cmd/manager/node_enrollment.go`
- Modify: `cmd/manager/node_enrollment_http_test.go`

- [ ] **Step 1: Write the failing install-script tests**

Append these tests to `cmd/manager/node_enrollment_http_test.go`:

```go
// 同时将 import 补充为:
// import (
//   "encoding/json"
//   "net/http"
//   "net/http/httptest"
//   "strings"
//   "testing"
//   "time"
//
//   "multi_node_platform/pkg/models"
// )

func TestGetNodeEnrollmentInstallScript(t *testing.T) {
	router := setupManagerTestRouter(t)

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

	okReq := httptest.NewRequest(http.MethodGet, "/api/v1/node-enrollments/enroll-1/install.sh?token=secret-token", nil)
	okReq.SetBasicAuth("admin", "admin")
	okRec := httptest.NewRecorder()
	router.ServeHTTP(okRec, okReq)

	if okRec.Code != http.StatusOK {
		t.Fatalf("script status = %d, body = %s", okRec.Code, okRec.Body.String())
	}
	body := okRec.Body.String()
	if !strings.Contains(body, "NODE_ENROLLMENT_ID=enroll-1") {
		t.Fatalf("script missing enrollment id: %s", body)
	}
	if !strings.Contains(body, "NODE_ENROLLMENT_TOKEN=secret-token") {
		t.Fatalf("script missing enrollment token: %s", body)
	}
	if !strings.Contains(body, "MANAGER_IP=manager.example.com:50051") {
		t.Fatalf("script missing manager grpc addr: %s", body)
	}

	badReq := httptest.NewRequest(http.MethodGet, "/api/v1/node-enrollments/enroll-1/install.sh?token=wrong-token", nil)
	badReq.SetBasicAuth("admin", "admin")
	badRec := httptest.NewRecorder()
	router.ServeHTTP(badRec, badReq)

	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid token status = %d, body = %s", badRec.Code, badRec.Body.String())
	}
}
```

- [ ] **Step 2: Run the install-script tests to verify they fail**

Run:

```bash
go test ./cmd/manager -run TestGetNodeEnrollmentInstallScript -v
```

Expected: FAIL because the install-script endpoint and script renderer do not exist yet.

- [ ] **Step 3: Implement install-script rendering and token checks**

Add the install-script renderer and reusable enrollment lookup to `cmd/manager/node_enrollment.go`:

```go
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
	displayName := shellQuoteEnv(enrollment.DisplayName)
	managerHTTPAddr := shellQuoteEnv(enrollment.ManagerHTTPAddr)
	managerGrpcAddr := shellQuoteEnv(enrollment.ManagerGrpcAddr)
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
mkdir -p /opt/mnp
curl -fsSL "%s/download/node" -o /opt/mnp/node
chmod +x /opt/mnp/node

cat >/etc/mnp-node.env <<'EOF'
MANAGER_IP=%s
NODE_DISPLAY_NAME=%s
NODE_ENROLLMENT_ID=%s
NODE_ENROLLMENT_TOKEN=%s
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
```

Add the HTTP handler in `cmd/manager/http_nodes.go`:

```go
func getNodeEnrollmentInstallScript(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 token"})
		return
	}

	enrollment, err := findUsableEnrollment(c.Param("id"), token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.String(http.StatusOK, renderNodeInstallScript(enrollment, token))
}
```

Register the route in `cmd/manager/http_server.go`:

```go
	api.GET("/node-enrollments/:id/install.sh", getNodeEnrollmentInstallScript)
```

- [ ] **Step 4: Run the install-script tests to verify they pass**

Run:

```bash
go test ./cmd/manager -run TestGetNodeEnrollmentInstallScript -v
```

Expected: PASS with script content assertions and invalid-token rejection.

- [ ] **Step 5: Commit the install-script endpoint**

```bash
git add cmd/manager/http_nodes.go cmd/manager/node_enrollment.go cmd/manager/node_enrollment_http_test.go cmd/manager/http_server.go
git commit -m "feat: add node enrollment install script"
```

## Task 3: Enforce Enrollment During gRPC Register

**Files:**
- Create: `cmd/manager/grpc_register_test.go`
- Modify: `api/proto/platform.proto`
- Modify: `api/pb/platform.pb.go`
- Modify: `api/pb/platform_grpc.pb.go`
- Modify: `cmd/manager/grpc_server.go`
- Modify: `cmd/manager/node_enrollment.go`

- [ ] **Step 1: Write the failing gRPC registration tests**

Create `cmd/manager/grpc_register_test.go`:

```go
package main

import (
	"context"
	"net"
	"testing"
	"time"

	pb "multi_node_platform/api/pb"
	"multi_node_platform/pkg/models"

	"google.golang.org/grpc/peer"
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
		NodeId:           "node-host-b",
		Hostname:         "host-b",
		DisplayName:      "",
		Version:          "v2.0.0",
		Capabilities:     []string{"docker", "browser"},
		EnrollmentId:     "enroll-1",
		EnrollmentToken:  "secret-token",
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
```

- [ ] **Step 2: Run the gRPC tests to verify they fail**

Run:

```bash
go test ./cmd/manager -run 'TestRegisterRequiresEnrollment|TestRegisterConsumesValidEnrollment' -v
```

Expected: FAIL because `RegisterRequest` does not carry enrollment fields and `GrpcServer.Register` still allows anonymous registration.

- [ ] **Step 3: Update protobuf and registration logic**

Extend `api/proto/platform.proto`:

```proto
message RegisterRequest {
    string node_id = 1;
    string hostname = 2;
    string version = 3;
    repeated string capabilities = 4;
    string display_name = 5;
    string enrollment_id = 6;
    string enrollment_token = 7;
}
```

Regenerate protobuf code:

```bash
protoc --go_out=. --go-grpc_out=. api/proto/platform.proto
```

Reuse `findUsableEnrollment` in `cmd/manager/node_enrollment.go` and add a consume helper:

```go
func consumeEnrollment(enrollment *models.NodeEnrollment, nodeID string) error {
	now := time.Now().UTC()
	return models.DB.Model(enrollment).Updates(map[string]interface{}{
		"used_at":         &now,
		"used_by_node_id": nodeID,
	}).Error
}
```

Update `cmd/manager/grpc_server.go`:

```go
func (s *GrpcServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	log.Printf("收到节点注册: ID=%s, Hostname=%s", req.NodeId, req.Hostname)

	if req.EnrollmentId == "" || req.EnrollmentToken == "" {
		return &pb.RegisterResponse{Success: false, Message: "节点未授权接入"}, nil
	}

	enrollment, err := findUsableEnrollment(req.EnrollmentId, req.EnrollmentToken)
	if err != nil {
		return &pb.RegisterResponse{Success: false, Message: err.Error()}, nil
	}

	token := uuid.New().String()
	nodeIP := ""
	if p, ok := peer.FromContext(ctx); ok {
		addr := p.Addr.String()
		if host, _, err := net.SplitHostPort(addr); err == nil {
			nodeIP = host
		}
	}

	displayName := req.DisplayName
	if displayName == "" {
		displayName = enrollment.DisplayName
	}
	if displayName == "" {
		displayName = req.Hostname
	}

	node := models.Node{
		ID:              req.NodeId,
		DisplayName:     displayName,
		Hostname:        req.Hostname,
		IP:              nodeIP,
		Token:           token,
		Capabilities:    fmt.Sprintf("%v", req.Capabilities),
		Version:         req.Version,
		OnlineState:     "online",
		LastHeartbeatAt: time.Now(),
	}
	if err := models.DB.Save(&node).Error; err != nil {
		log.Printf("节点保存失败: %v", err)
		return &pb.RegisterResponse{Success: false, Message: "Internal Server Error"}, nil
	}
	if err := consumeEnrollment(enrollment, req.NodeId); err != nil {
		log.Printf("接入授权标记失败: %v", err)
		return &pb.RegisterResponse{Success: false, Message: "接入授权写入失败"}, nil
	}

	return &pb.RegisterResponse{Success: true, Token: token, Message: "注册成功"}, nil
}
```

- [ ] **Step 4: Run the gRPC tests to verify they pass**

Run:

```bash
go test ./cmd/manager -run 'TestRegisterRequiresEnrollment|TestRegisterConsumesValidEnrollment' -v
```

Expected: PASS with explicit anonymous-registration rejection and enrollment consumption.

- [ ] **Step 5: Commit the gRPC enrollment enforcement**

```bash
git add api/proto/platform.proto api/pb/platform.pb.go api/pb/platform_grpc.pb.go cmd/manager/grpc_server.go cmd/manager/grpc_register_test.go cmd/manager/node_enrollment.go
git commit -m "feat: require enrollment for node registration"
```

## Task 4: Make the Node Binary Enrollment-Aware

**Files:**
- Create: `cmd/node/config.go`
- Create: `cmd/node/config_test.go`
- Modify: `cmd/node/main.go`

- [ ] **Step 1: Write the failing node config tests**

Create `cmd/node/config_test.go`:

```go
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
```

- [ ] **Step 2: Run the node config tests to verify they fail**

Run:

```bash
go test ./cmd/node -run 'TestLoadNodeConfigUsesEnrollmentEnv|TestNormalizeManagerAddrAddsDefaultPort' -v
```

Expected: FAIL because `loadNodeConfig` and `normalizeManagerAddr` do not exist.

- [ ] **Step 3: Implement node config loading and registration fields**

Create `cmd/node/config.go`:

```go
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
		Hostname:        hostname,
	}
}
```

Refactor the top of `cmd/node/main.go` to use `NodeConfig`:

```go
var (
	nodeID = ""
	token  = ""
	config NodeConfig
)

func main() {
	config = loadNodeConfig()
	nodeID = config.NodeID

	negotiateDockerAPI()

	conn, err := grpc.Dial(config.ManagerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("连接 Manager 失败: %v", err)
	}
	defer conn.Close()

	c := pb.NewNodeServiceClient(conn)
	ctx := context.Background()

	registerRes, err := c.Register(ctx, &pb.RegisterRequest{
		NodeId:          config.NodeID,
		Hostname:        config.Hostname,
		DisplayName:     config.DisplayName,
		Version:         "v2.0.0",
		Capabilities:    []string{"docker", "tun2socks", "tun2proxy", "browser"},
		EnrollmentId:    config.EnrollmentID,
		EnrollmentToken: config.EnrollmentToken,
	})
	if err != nil {
		log.Fatalf("注册失败: %v", err)
	}
	if !registerRes.Success {
		log.Fatalf("注册被拒: %s", registerRes.Message)
	}
	token = registerRes.Token
	log.Printf("注册成功, token: %s", token)
```

- [ ] **Step 4: Run the node config tests to verify they pass**

Run:

```bash
go test ./cmd/node -run 'TestLoadNodeConfigUsesEnrollmentEnv|TestNormalizeManagerAddrAddsDefaultPort' -v
```

Expected: PASS with coverage for manager-address normalization and enrollment env parsing.

- [ ] **Step 5: Commit the node config changes**

```bash
git add cmd/node/config.go cmd/node/config_test.go cmd/node/main.go
git commit -m "feat: load node enrollment config from env"
```

## Task 5: Add Node Enrollment UI and Frontend E2E Coverage

**Files:**
- Create: `web/tests/node-enrollment.spec.js`
- Modify: `web/src/views/NodeList.vue`
- Modify: `web/package.json`

- [ ] **Step 1: Write the failing Playwright test for the Nodes page**

Create `web/tests/node-enrollment.spec.js`:

```javascript
import { test, expect } from '@playwright/test';

test('creates a node enrollment and shows the install command', async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('auth_token', btoa('admin:admin'));
  });

  await page.route('**/api/v1/nodes', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: [] }),
    });
  });

  await page.route('**/api/v1/node-enrollments', async route => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ data: [] }),
      });
      return;
    }

    const payload = JSON.parse(route.request().postData() || '{}');
    if (payload.display_name !== 'tokyo-01') {
      throw new Error(`unexpected payload: ${JSON.stringify(payload)}`);
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          id: 'enroll-1',
          expires_at: '2026-04-02T12:00:00Z',
          install_command: "curl -fsSL 'https://manager.example.com/api/v1/node-enrollments/enroll-1/install.sh?token=plain-token' | sudo bash",
        },
      }),
    });
  });

  await page.goto('/nodes');
  await page.getByTestId('open-node-enrollment').click();
  await page.locator('[data-testid="node-enrollment-display-name"] input').fill('tokyo-01');
  await page.locator('[data-testid="node-enrollment-http"] input').fill('https://manager.example.com');
  await page.locator('[data-testid="node-enrollment-grpc"] input').fill('manager.example.com:50051');
  await page.getByTestId('submit-node-enrollment').click();

  await expect(page.getByTestId('node-enrollment-command')).toContainText('/api/v1/node-enrollments/enroll-1/install.sh?token=plain-token');
  await expect(page.getByText('2026-04-02')).toBeVisible();
});
```

- [ ] **Step 2: Run the Playwright test to verify it fails**

Run:

```bash
cd web && npx playwright test tests/node-enrollment.spec.js --project=chromium
```

Expected: FAIL because the Nodes page has no add-enrollment button, dialog, or `data-testid` hooks yet.

- [ ] **Step 3: Implement the enrollment dialog, result view, and history table**

Add a Playwright script to `web/package.json`:

```json
{
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview",
    "test:e2e": "playwright test"
  }
}
```

Update `web/src/views/NodeList.vue` template with a toolbar, dialog, result state, and history table:

```vue
<template>
  <div>
    <div class="page-header">
      <h2>节点管理</h2>
      <el-button type="primary" data-testid="open-node-enrollment" @click="openEnrollmentDialog">
        添加节点
      </el-button>
    </div>

    <el-table :data="nodes" border style="width: 100%">
      <el-table-column prop="id" label="节点ID" width="200">
        <template #default="{ row }">{{ row.id.substring(0, 16) }}...</template>
      </el-table-column>
      <el-table-column prop="display_name" label="显示名" width="140" />
      <el-table-column prop="hostname" label="主机名" width="140" />
      <el-table-column prop="ip" label="IP" width="130" />
      <el-table-column prop="online_state" label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.online_state === 'online' ? 'success' : 'danger'">
            {{ row.online_state === 'online' ? '在线' : '离线' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="version" label="版本" width="80" />
      <el-table-column prop="max_groups" label="最大组数" width="90" />
      <el-table-column prop="last_heartbeat_at" label="最近心跳" />
      <el-table-column label="操作" width="150">
        <template #default="{ row }">
          <el-button size="small" @click="editNode(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="delNode(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <h3 style="margin-top: 24px;">接入记录</h3>
    <el-table :data="enrollments" border style="width: 100%">
      <el-table-column prop="display_name" label="备注名" min-width="160" />
      <el-table-column prop="manager_http_addr" label="HTTP 地址" min-width="220" />
      <el-table-column prop="manager_grpc_addr" label="gRPC 地址" min-width="180" />
      <el-table-column prop="status" label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'used' ? 'success' : row.status === 'expired' ? 'danger' : 'warning'">
            {{ row.status === 'used' ? '已使用' : row.status === 'expired' ? '已过期' : '待使用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" min-width="180" />
      <el-table-column prop="expires_at" label="过期时间" min-width="180" />
      <el-table-column prop="used_at" label="使用时间" min-width="180" />
      <el-table-column prop="used_by_node_id" label="已绑定节点" min-width="180" />
    </el-table>

    <el-dialog v-model="enrollmentVisible" title="添加节点" width="640px">
      <template v-if="!enrollmentResult">
        <el-form :model="enrollmentForm" label-width="140px">
          <el-form-item label="节点备注">
            <div data-testid="node-enrollment-display-name">
              <el-input v-model="enrollmentForm.display_name" />
            </div>
          </el-form-item>
          <el-form-item label="Manager HTTP 地址">
            <div data-testid="node-enrollment-http">
              <el-input v-model="enrollmentForm.manager_http_addr" />
            </div>
          </el-form-item>
          <el-form-item label="Manager gRPC 地址">
            <div data-testid="node-enrollment-grpc">
              <el-input v-model="enrollmentForm.manager_grpc_addr" />
            </div>
          </el-form-item>
          <el-alert
            type="info"
            :closable="false"
            title="安装命令仅展示一次，授权 24 小时有效且只能使用一次，目标服务器需支持 systemd。"
          />
        </el-form>
      </template>

      <template v-else>
        <el-alert type="success" :closable="false" title="安装命令已生成，请尽快复制到目标 VPS 执行。" />
        <p>过期时间：{{ enrollmentResult.expires_at }}</p>
        <el-input
          type="textarea"
          :rows="5"
          readonly
          :model-value="enrollmentResult.install_command"
          data-testid="node-enrollment-command"
        />
      </template>

      <template #footer>
        <el-button @click="enrollmentVisible = false">关闭</el-button>
        <el-button
          v-if="!enrollmentResult"
          type="primary"
          data-testid="submit-node-enrollment"
          @click="createEnrollment"
        >
          生成安装命令
        </el-button>
        <el-button v-else type="primary" @click="copyInstallCommand">
          复制命令
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editVisible" title="编辑节点" width="400px">
      <el-form :model="editForm" label-width="100px">
        <el-form-item label="显示名称"><el-input v-model="editForm.display_name" /></el-form-item>
        <el-form-item label="最大组数"><el-input-number v-model="editForm.max_groups" :min="1" :max="500" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" @click="saveNode">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>
```

Replace the entire `<script setup>` block in `web/src/views/NodeList.vue` with:

```js
import { ref, onMounted } from 'vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'

const nodes = ref([])
const editVisible = ref(false)
const editId = ref('')
const editForm = ref({ display_name: '', max_groups: 50 })
const enrollments = ref([])
const enrollmentVisible = ref(false)
const enrollmentResult = ref(null)
const enrollmentForm = ref({
  display_name: '',
  manager_http_addr: window.location.origin,
  manager_grpc_addr: `${window.location.hostname}:50051`,
})

const fetchNodes = async () => {
  const res = await axios.get('/api/v1/nodes')
  nodes.value = res.data.data || []
}

const editNode = (row) => {
  editId.value = row.id
  editForm.value = { display_name: row.display_name, max_groups: row.max_groups || 50 }
  editVisible.value = true
}

const saveNode = async () => {
  try {
    await axios.put(`/api/v1/nodes/${editId.value}`, editForm.value)
    ElMessage.success('已更新')
    editVisible.value = false
    await fetchNodes()
  } catch (e) {
    ElMessage.error('更新失败')
  }
}

const delNode = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除节点 ${row.display_name}？`)
    await axios.delete(`/api/v1/nodes/${row.id}`)
    ElMessage.success('已删除')
    await fetchNodes()
  } catch (e) {
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '删除失败')
    }
  }
}

const fetchEnrollments = async () => {
  const res = await axios.get('/api/v1/node-enrollments')
  enrollments.value = res.data.data || []
}

const openEnrollmentDialog = () => {
  enrollmentResult.value = null
  enrollmentForm.value = {
    display_name: '',
    manager_http_addr: window.location.origin,
    manager_grpc_addr: `${window.location.hostname}:50051`,
  }
  enrollmentVisible.value = true
}

const createEnrollment = async () => {
  try {
    const res = await axios.post('/api/v1/node-enrollments', enrollmentForm.value)
    enrollmentResult.value = res.data.data
    ElMessage.success('安装命令已生成')
    await fetchEnrollments()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '生成失败')
  }
}

const copyInstallCommand = async () => {
  try {
    await navigator.clipboard.writeText(enrollmentResult.value.install_command)
    ElMessage.success('安装命令已复制')
  } catch (e) {
    ElMessage.error('复制失败')
  }
}

onMounted(async () => {
  await Promise.all([fetchNodes(), fetchEnrollments()])
})
```

- [ ] **Step 4: Run the Playwright test to verify it passes**

Run:

```bash
cd web && npm run test:e2e -- tests/node-enrollment.spec.js --project=chromium
```

Expected: PASS with one green scenario covering dialog open, create request, and install-command rendering.

- [ ] **Step 5: Commit the frontend enrollment flow**

```bash
git add web/src/views/NodeList.vue web/tests/node-enrollment.spec.js web/package.json
git commit -m "feat: add node enrollment flow to nodes page"
```

## Task 6: Format, Verify, and Smoke-Check the Full Flow

**Files:**
- Modify: `cmd/manager/http_nodes.go`
- Modify: `cmd/manager/node_enrollment.go`
- Modify: `cmd/manager/grpc_server.go`
- Modify: `cmd/node/config.go`
- Modify: `cmd/node/main.go`
- Modify: `web/src/views/NodeList.vue`
- Modify: `web/tests/node-enrollment.spec.js`
- Modify: `web/package.json`
- Modify: `pkg/models/models.go`
- Modify: `pkg/models/db.go`
- Modify: `api/proto/platform.proto`
- Modify: `api/pb/platform.pb.go`
- Modify: `api/pb/platform_grpc.pb.go`

- [ ] **Step 1: Format all touched Go files**

Run:

```bash
gofmt -w cmd/manager/http_nodes.go cmd/manager/node_enrollment.go cmd/manager/node_enrollment_http_test.go cmd/manager/grpc_register_test.go cmd/manager/test_helpers_test.go cmd/manager/grpc_server.go cmd/node/config.go cmd/node/config_test.go cmd/node/main.go pkg/models/models.go pkg/models/db.go
```

Expected: no output, files rewritten in place.

- [ ] **Step 2: Run the targeted Go test suites**

Run:

```bash
go test ./cmd/manager ./cmd/node -v
```

Expected: PASS, including enrollment HTTP tests, gRPC register tests, and node config tests.

- [ ] **Step 3: Build the frontend and rerun the targeted E2E test**

Run:

```bash
cd web && npm run build && npm run test:e2e -- tests/node-enrollment.spec.js --project=chromium
```

Expected: Vite build succeeds, then Playwright passes the node enrollment scenario.

- [ ] **Step 4: Smoke-check the install script response manually**

Run:

```bash
curl -sS -u admin:admin "http://127.0.0.1:8002/api/v1/node-enrollments/<enrollment-id>/install.sh?token=<plain-token>" | sed -n '1,80p'
```

Expected: shell script starts with `#!/usr/bin/env bash` and includes `MANAGER_IP=`, `NODE_ENROLLMENT_ID=`, and `NODE_ENROLLMENT_TOKEN=`.

- [ ] **Step 5: Commit the verification pass**

```bash
git add api/proto/platform.proto api/pb/platform.pb.go api/pb/platform_grpc.pb.go cmd/manager/http_nodes.go cmd/manager/node_enrollment.go cmd/manager/node_enrollment_http_test.go cmd/manager/grpc_register_test.go cmd/manager/test_helpers_test.go cmd/manager/grpc_server.go cmd/node/config.go cmd/node/config_test.go cmd/node/main.go pkg/models/models.go pkg/models/db.go web/src/views/NodeList.vue web/tests/node-enrollment.spec.js web/package.json go.mod go.sum
git commit -m "test: verify node enrollment flow"
```
