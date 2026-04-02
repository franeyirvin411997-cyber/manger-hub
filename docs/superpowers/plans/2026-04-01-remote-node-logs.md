# Remote Node Logs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add manager-mediated remote log viewing so the Nodes page can stream `mnp-node` service logs and whitelisted container logs in real time, while separately showing the latest 200 historical lines.

**Architecture:** Keep the existing network direction where nodes initiate connections to manager. Add a long-lived bidirectional `LogStream` gRPC channel from each node to manager, let manager publish in-memory log subscriptions over that stream, and let nodes push `status/live/history/error` events back for `text/event-stream` fan-out to browsers. Extend the Nodes page with a log dialog that opens an authenticated `fetch` stream, parses event frames in the browser, renders live and history panes separately, and controls start/stop/reconnect.

**Tech Stack:** Go 1.25, Gin, gRPC/protobuf, Vue 3, Element Plus, Playwright, SQLite-backed Go tests

---

## File Structure

- Create: `cmd/manager/node_log_hub.go`
  - In-memory registry for online node log streams and browser subscriptions
- Create: `cmd/manager/node_log_hub_test.go`
  - Unit tests for stream binding, subscription routing, and cleanup
- Create: `cmd/manager/http_node_logs.go`
  - `text/event-stream` endpoint and query validation for remote logs
- Create: `cmd/manager/http_node_logs_test.go`
  - HTTP tests for stream access control and bad parameter handling
- Create: `cmd/node/log_stream.go`
  - Node-side long-lived log stream loop, whitelist validation, command runners
- Create: `cmd/node/log_stream_test.go`
  - Unit tests for command building and whitelist enforcement
- Create: `web/tests/node-logs.spec.js`
  - Playwright test for the Nodes-page log dialog using a mocked streaming `fetch`
- Modify: `api/proto/platform.proto`
  - Add `LogStream` RPC and `LogEnvelope` message
- Modify: `api/pb/platform.pb.go`
  - Regenerated protobuf types
- Modify: `api/pb/platform_grpc.pb.go`
  - Regenerated gRPC stubs
- Modify: `cmd/manager/grpc_server.go`
  - Implement `LogStream`, bind node streams, and release them on disconnect
- Modify: `cmd/manager/http_server.go`
  - Register the authenticated node log stream route
- Modify: `cmd/node/main.go`
  - Start the persistent log stream loop after successful registration
- Modify: `web/src/views/NodeList.vue`
  - Add the log dialog, source selection, authenticated `fetch` stream lifecycle, and split panes

## Task 1: Protobuf and Manager Log Hub Foundation

**Files:**
- Create: `cmd/manager/node_log_hub.go`
- Create: `cmd/manager/node_log_hub_test.go`
- Modify: `api/proto/platform.proto`
- Modify: `api/pb/platform.pb.go`
- Modify: `api/pb/platform_grpc.pb.go`

- [ ] **Step 1: Write the failing manager log-hub tests**

Create `cmd/manager/node_log_hub_test.go`:

```go
package main

import (
	"testing"
	"time"

	pb "multi_node_platform/api/pb"
)

func TestNodeLogHubCreatesSubscriptionForOnlineNode(t *testing.T) {
	hub := newNodeLogHub()
	stream := newTestNodeLogStream()
	hub.bindNodeStream("node-a", "token-a", stream)

	sub, err := hub.createSubscription("node-a", "node_service", "")
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if sub.ID == "" {
		t.Fatal("expected subscription id")
	}

	msg := <-stream.sent
	if msg.MessageType != "subscribe_request" {
		t.Fatalf("message_type = %s", msg.MessageType)
	}
	if msg.SubscriptionId != sub.ID {
		t.Fatalf("subscription id = %s", msg.SubscriptionId)
	}
	if msg.SourceType != "node_service" {
		t.Fatalf("source type = %s", msg.SourceType)
	}
}

func TestNodeLogHubRoutesChunksToSubscriber(t *testing.T) {
	hub := newNodeLogHub()
	stream := newTestNodeLogStream()
	hub.bindNodeStream("node-a", "token-a", stream)

	sub, err := hub.createSubscription("node-a", "container", "app-demo")
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	chunk := &pb.LogEnvelope{
		SubscriptionId: sub.ID,
		MessageType:    "live",
		StreamType:     "live",
		Content:        "hello",
		TimestampUnix:  time.Now().Unix(),
	}
	if err := hub.publishChunk(chunk); err != nil {
		t.Fatalf("publish chunk: %v", err)
	}

	got := <-sub.Events
	if got.Content != "hello" {
		t.Fatalf("content = %s", got.Content)
	}
}

type testNodeLogStream struct {
	sent chan *pb.LogEnvelope
}

func newTestNodeLogStream() *testNodeLogStream {
	return &testNodeLogStream{sent: make(chan *pb.LogEnvelope, 8)}
}

func (s *testNodeLogStream) Send(msg *pb.LogEnvelope) error {
	s.sent <- msg
	return nil
}
```

- [ ] **Step 2: Run the log-hub tests to verify they fail**

Run:

```bash
go test ./cmd/manager -run 'TestNodeLogHubCreatesSubscriptionForOnlineNode|TestNodeLogHubRoutesChunksToSubscriber' -v
```

Expected: FAIL because `LogEnvelope`, `newNodeLogHub`, and related helpers do not exist.

- [ ] **Step 3: Add protobuf types and implement the manager log hub**

Extend `api/proto/platform.proto`:

```proto
message LogEnvelope {
    string message_type = 1;
    string subscription_id = 2;
    string node_id = 3;
    string token = 4;
    string source_type = 5;
    string container_name = 6;
    string stream_type = 7;
    string content = 8;
    int64 timestamp_unix = 9;
}

service NodeService {
    rpc Register(RegisterRequest) returns (RegisterResponse);
    rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
    rpc ReportTaskResult(TaskResult) returns (TaskResultResponse);
    rpc LogStream(stream LogEnvelope) returns (stream LogEnvelope);
}
```

Regenerate stubs:

```bash
protoc --plugin=protoc-gen-go=/root/go/bin/protoc-gen-go --plugin=protoc-gen-go-grpc=/root/go/bin/protoc-gen-go-grpc --go_out=. --go_opt=module=multi_node_platform --go-grpc_out=. --go-grpc_opt=module=multi_node_platform api/proto/platform.proto
```

Create `cmd/manager/node_log_hub.go`:

```go
package main

import (
	"fmt"
	"sync"
	"time"

	pb "multi_node_platform/api/pb"
)

type nodeLogStreamSender interface {
	Send(*pb.LogEnvelope) error
}

type nodeLogSubscription struct {
	ID            string
	NodeID        string
	SourceType    string
	ContainerName string
	Events        chan *pb.LogEnvelope
}

type nodeLogHub struct {
	mu            sync.RWMutex
	nodeStreams   map[string]nodeLogStreamSender
	nodeTokens    map[string]string
	subscriptions map[string]*nodeLogSubscription
}

func newNodeLogHub() *nodeLogHub {
	return &nodeLogHub{
		nodeStreams:   map[string]nodeLogStreamSender{},
		nodeTokens:    map[string]string{},
		subscriptions: map[string]*nodeLogSubscription{},
	}
}

func (h *nodeLogHub) bindNodeStream(nodeID, token string, stream nodeLogStreamSender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nodeStreams[nodeID] = stream
	h.nodeTokens[nodeID] = token
}

func (h *nodeLogHub) unbindNodeStream(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.nodeStreams, nodeID)
	delete(h.nodeTokens, nodeID)
}

func (h *nodeLogHub) createSubscription(nodeID, sourceType, containerName string) (*nodeLogSubscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	stream, ok := h.nodeStreams[nodeID]
	if !ok {
		return nil, fmt.Errorf("节点日志通道未建立")
	}

	sub := &nodeLogSubscription{
		ID:            fmt.Sprintf("%d", time.Now().UnixNano()),
		NodeID:        nodeID,
		SourceType:    sourceType,
		ContainerName: containerName,
		Events:        make(chan *pb.LogEnvelope, 256),
	}
	h.subscriptions[sub.ID] = sub

	if err := stream.Send(&pb.LogEnvelope{
		MessageType:    "subscribe_request",
		SubscriptionId: sub.ID,
		NodeId:         nodeID,
		Token:          h.nodeTokens[nodeID],
		SourceType:     sourceType,
		ContainerName:  containerName,
		TimestampUnix:  time.Now().Unix(),
	}); err != nil {
		delete(h.subscriptions, sub.ID)
		return nil, err
	}

	return sub, nil
}

func (h *nodeLogHub) cancelSubscription(subscriptionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sub, ok := h.subscriptions[subscriptionID]
	if !ok {
		return
	}
	if stream, ok := h.nodeStreams[sub.NodeID]; ok {
		_ = stream.Send(&pb.LogEnvelope{
			MessageType:    "subscribe_cancel",
			SubscriptionId: subscriptionID,
			NodeId:         sub.NodeID,
			Token:          h.nodeTokens[sub.NodeID],
			TimestampUnix:  time.Now().Unix(),
		})
	}
	delete(h.subscriptions, subscriptionID)
	close(sub.Events)
}

func (h *nodeLogHub) publishChunk(msg *pb.LogEnvelope) error {
	h.mu.RLock()
	sub, ok := h.subscriptions[msg.SubscriptionId]
	h.mu.RUnlock()
	if !ok {
		return fmt.Errorf("日志订阅不存在")
	}
	sub.Events <- msg
	return nil
}
```

- [ ] **Step 4: Run the log-hub tests to verify they pass**

Run:

```bash
go test ./cmd/manager -run 'TestNodeLogHubCreatesSubscriptionForOnlineNode|TestNodeLogHubRoutesChunksToSubscriber' -v
```

Expected: PASS with one test covering subscription creation and one covering chunk fan-out.

- [ ] **Step 5: Commit the protobuf and hub foundation**

```bash
git add api/proto/platform.proto api/pb/platform.pb.go api/pb/platform_grpc.pb.go cmd/manager/node_log_hub.go cmd/manager/node_log_hub_test.go
git commit -m "feat: add node log stream protocol and hub"
```

## Task 2: Manager LogStream Binding and HTTP Stream Endpoint

**Files:**
- Create: `cmd/manager/http_node_logs.go`
- Create: `cmd/manager/http_node_logs_test.go`
- Modify: `cmd/manager/grpc_server.go`
- Modify: `cmd/manager/http_server.go`
- Modify: `cmd/manager/node_log_hub.go`

- [ ] **Step 1: Write the failing manager HTTP tests**

Create `cmd/manager/http_node_logs_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"multi_node_platform/pkg/models"
)

func TestNodeLogStreamRejectsOfflineNode(t *testing.T) {
	router := setupManagerTestRouter(t)
	models.DB.Create(&models.Node{
		ID:          "node-offline",
		DisplayName: "offline",
		Hostname:    "offline",
		Token:       "token-offline",
		OnlineState: "offline",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-offline/logs/stream?source_type=node_service", nil)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestNodeLogStreamRejectsInvalidContainerName(t *testing.T) {
	router := setupManagerTestRouter(t)
	models.DB.Create(&models.Node{
		ID:              "node-online",
		DisplayName:     "online",
		Hostname:        "online",
		Token:           "token-online",
		OnlineState:     "online",
		LastHeartbeatAt: time.Now(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-online/logs/stream?source_type=container&container_name=mysql", nil)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the manager HTTP tests to verify they fail**

Run:

```bash
go test ./cmd/manager -run 'TestNodeLogStreamRejectsOfflineNode|TestNodeLogStreamRejectsInvalidContainerName' -v
```

Expected: FAIL because the log stream route and validation helpers do not exist.

- [ ] **Step 3: Implement `LogStream`, node authentication, and the HTTP stream endpoint**

Initialize a package-level hub in `cmd/manager/node_log_hub.go`:

```go
var managerNodeLogHub = newNodeLogHub()
```

Add validation and stream handling in `cmd/manager/http_node_logs.go`:

```go
package main

import (
	"fmt"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"multi_node_platform/pkg/models"
)

func isAllowedContainerName(name string) bool {
	return strings.HasPrefix(name, "tunnel-") || strings.HasPrefix(name, "app-") || strings.HasPrefix(name, "browser-")
}

func validateNodeLogQuery(sourceType, containerName string) error {
	switch sourceType {
	case "node_service":
		return nil
	case "container":
		if containerName == "" {
			return fmt.Errorf("container_name 不能为空")
		}
		if !isAllowedContainerName(containerName) {
			return fmt.Errorf("container_name 不在允许范围")
		}
		return nil
	default:
		return fmt.Errorf("source_type 无效")
	}
}

func streamNodeLogs(c *gin.Context) {
	nodeID := c.Param("id")
	sourceType := c.Query("source_type")
	containerName := c.Query("container_name")

	if err := validateNodeLogQuery(sourceType, containerName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var node models.Node
	if err := models.DB.Where("id = ?", nodeID).First(&node).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
		return
	}
	if node.OnlineState != "online" {
		c.JSON(http.StatusConflict, gin.H{"error": "节点不在线"})
		return
	}

	sub, err := managerNodeLogHub.createSubscription(nodeID, sourceType, containerName)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	defer managerNodeLogHub.cancelSubscription(sub.ID)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.Flush()

	notify := c.Request.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg, ok := <-sub.Events:
			if !ok {
				return
			}
			payload, _ := json.Marshal(gin.H{
				"type":      msg.StreamType,
				"content":   msg.Content,
				"timestamp": time.Unix(msg.TimestampUnix, 0).UTC().Format(time.RFC3339),
			})
			c.SSEvent(msg.StreamType, string(payload))
			c.Writer.Flush()
		}
	}
}
```

Register the route in `cmd/manager/http_server.go`:

```go
	api.GET("/nodes/:id/logs/stream", streamNodeLogs)
```

Implement `LogStream` in `cmd/manager/grpc_server.go`:

```go
func (s *GrpcServer) LogStream(stream pb.NodeService_LogStreamServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if first.MessageType != "stream_open" {
		return status.Error(codes.InvalidArgument, "missing stream_open")
	}

	var node models.Node
	if err := models.DB.Where("id = ? AND token = ?", first.NodeId, first.Token).First(&node).Error; err != nil {
		return status.Error(codes.Unauthenticated, "未授权的节点或Token无效")
	}

	managerNodeLogHub.bindNodeStream(first.NodeId, first.Token, stream)
	defer managerNodeLogHub.unbindNodeStream(first.NodeId)

	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if msg.SubscriptionId == "" {
			continue
		}
		if err := managerNodeLogHub.publishChunk(msg); err != nil {
			log.Printf("日志块分发失败: %v", err)
		}
	}
}
```

- [ ] **Step 4: Run the manager HTTP tests to verify they pass**

Run:

```bash
go test ./cmd/manager -run 'TestNodeLogStreamRejectsOfflineNode|TestNodeLogStreamRejectsInvalidContainerName' -v
```

Expected: PASS with explicit 409 for offline nodes and 400 for bad container names.

- [ ] **Step 5: Commit the manager log transport**

```bash
git add cmd/manager/http_node_logs.go cmd/manager/http_node_logs_test.go cmd/manager/grpc_server.go cmd/manager/http_server.go cmd/manager/node_log_hub.go
git commit -m "feat: add manager node log streaming"
```

## Task 3: Node Log Stream Loop and Whitelisted Command Execution

**Files:**
- Create: `cmd/node/log_stream.go`
- Create: `cmd/node/log_stream_test.go`
- Modify: `cmd/node/main.go`

- [ ] **Step 1: Write the failing node log-stream tests**

Create `cmd/node/log_stream_test.go`:

```go
package main

import "testing"

func TestBuildContainerLogCommandsRejectsInvalidContainerName(t *testing.T) {
	_, _, err := buildLogCommands("container", "mysql")
	if err == nil {
		t.Fatal("expected invalid container name error")
	}
}

func TestBuildNodeServiceLogCommands(t *testing.T) {
	live, history, err := buildLogCommands("node_service", "")
	if err != nil {
		t.Fatalf("build commands: %v", err)
	}
	if live[0] != "journalctl" || history[0] != "journalctl" {
		t.Fatalf("unexpected commands: %v / %v", live, history)
	}
}

func TestBuildContainerLogCommands(t *testing.T) {
	live, history, err := buildLogCommands("container", "app-demo")
	if err != nil {
		t.Fatalf("build commands: %v", err)
	}
	if live[0] != "docker" || history[0] != "docker" {
		t.Fatalf("unexpected commands: %v / %v", live, history)
	}
}
```

- [ ] **Step 2: Run the node log-stream tests to verify they fail**

Run:

```bash
go test ./cmd/node -run 'TestBuildContainerLogCommandsRejectsInvalidContainerName|TestBuildNodeServiceLogCommands|TestBuildContainerLogCommands' -v
```

Expected: FAIL because the log-command builders do not exist.

- [ ] **Step 3: Implement the node log stream loop and command builders**

Create `cmd/node/log_stream.go`:

```go
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	pb "multi_node_platform/api/pb"
)

func buildLogCommands(sourceType, containerName string) ([]string, []string, error) {
	switch sourceType {
	case "node_service":
		return []string{"journalctl", "-u", "mnp-node", "-f", "-n", "0", "--no-pager"},
			[]string{"journalctl", "-u", "mnp-node", "-n", "200", "--no-pager"},
			nil
	case "container":
		if !strings.HasPrefix(containerName, "tunnel-") && !strings.HasPrefix(containerName, "app-") && !strings.HasPrefix(containerName, "browser-") {
			return nil, nil, fmt.Errorf("container_name 不在允许范围")
		}
		return []string{"docker", "logs", "--tail", "0", "-f", containerName},
			[]string{"docker", "logs", "--tail", "200", containerName},
			nil
	default:
		return nil, nil, fmt.Errorf("source_type 无效")
	}
}

func sendLogEnvelope(stream pb.NodeService_LogStreamClient, msgType, subID, streamType, content string) error {
	return stream.Send(&pb.LogEnvelope{
		MessageType:    msgType,
		SubscriptionId: subID,
		NodeId:         config.NodeID,
		Token:          token,
		StreamType:     streamType,
		Content:        content,
		TimestampUnix:  time.Now().Unix(),
	})
}

func startLogStreamLoop(client pb.NodeServiceClient) {
	go func() {
		for {
			ctx := context.Background()
			stream, err := client.LogStream(ctx)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}
			_ = stream.Send(&pb.LogEnvelope{
				MessageType:   "stream_open",
				NodeId:        config.NodeID,
				Token:         token,
				TimestampUnix: time.Now().Unix(),
			})
			runLogStreamSession(stream)
			time.Sleep(2 * time.Second)
		}
	}()
}

func runLogStreamSession(stream pb.NodeService_LogStreamClient) {
	var mu sync.Mutex
	cancelMap := map[string]context.CancelFunc{}

	for {
		msg, err := stream.Recv()
		if err != nil {
			return
		}

		switch msg.MessageType {
		case "subscribe_request":
			live, history, err := buildLogCommands(msg.SourceType, msg.ContainerName)
			if err != nil {
				_ = sendLogEnvelope(stream, "error", msg.SubscriptionId, "error", err.Error())
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			mu.Lock()
			cancelMap[msg.SubscriptionId] = cancel
			mu.Unlock()
			go runLogSubscription(ctx, stream, msg.SubscriptionId, live, history)
		case "subscribe_cancel":
			mu.Lock()
			cancel := cancelMap[msg.SubscriptionId]
			delete(cancelMap, msg.SubscriptionId)
			mu.Unlock()
			if cancel != nil {
				cancel()
			}
		}
	}
}

func runLogSubscription(ctx context.Context, stream pb.NodeService_LogStreamClient, subscriptionID string, liveCmd, historyCmd []string) {
	_ = sendLogEnvelope(stream, "status", subscriptionID, "status", "live_connecting")
	if err := startLiveCommand(ctx, stream, subscriptionID, liveCmd); err != nil {
		_ = sendLogEnvelope(stream, "error", subscriptionID, "error", err.Error())
		return
	}
	_ = sendLogEnvelope(stream, "status", subscriptionID, "status", "live_connected")
	_ = emitHistory(stream, subscriptionID, historyCmd)
}

func emitHistory(stream pb.NodeService_LogStreamClient, subscriptionID string, args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return sendLogEnvelope(stream, "history", subscriptionID, "history", "暂无最近历史日志")
	}
	for _, line := range lines {
		_ = sendLogEnvelope(stream, "history", subscriptionID, "history", line)
	}
	return nil
}

func startLiveCommand(ctx context.Context, stream pb.NodeService_LogStreamClient, subscriptionID string, args []string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go scanLogPipe(stream, subscriptionID, stdout)
	go scanLogPipe(stream, subscriptionID, stderr)
	go func() {
		_ = cmd.Wait()
		_ = sendLogEnvelope(stream, "status", subscriptionID, "status", "stream_closed")
	}()
	return nil
}

func scanLogPipe(stream pb.NodeService_LogStreamClient, subscriptionID string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		_ = sendLogEnvelope(stream, "live", subscriptionID, "live", scanner.Text())
	}
}
```

Start the loop in `cmd/node/main.go` right after successful registration:

```go
	log.Printf("注册成功, token: %s", token)
	startLogStreamLoop(c)
```

- [ ] **Step 4: Run the node log-stream tests to verify they pass**

Run:

```bash
go test ./cmd/node -run 'TestBuildContainerLogCommandsRejectsInvalidContainerName|TestBuildNodeServiceLogCommands|TestBuildContainerLogCommands' -v
```

Expected: PASS with command-building coverage for both source types and whitelist enforcement.

- [ ] **Step 5: Commit the node log stream client**

```bash
git add cmd/node/log_stream.go cmd/node/log_stream_test.go cmd/node/main.go
git commit -m "feat: add node log stream client"
```

## Task 4: Nodes-Page Log Dialog and Authenticated Fetch Wiring

**Files:**
- Create: `web/tests/node-logs.spec.js`
- Modify: `web/src/views/NodeList.vue`

- [ ] **Step 1: Write the failing Playwright test for the log dialog**

Create `web/tests/node-logs.spec.js`:

```javascript
import { test, expect } from '@playwright/test';

test('opens node logs dialog and renders live plus history sections', async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('auth_token', btoa('admin:admin'));
    window.__originalFetch = window.fetch.bind(window);
    window.fetch = async (url) => {
      if (typeof url === 'string' && url.includes('/logs/stream')) {
        const encoder = new TextEncoder();
        return new Response(new ReadableStream({
          start(controller) {
            window.__pushStreamChunk = (chunk) => controller.enqueue(encoder.encode(chunk));
          }
        }), {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' }
        });
      }
      return window.__originalFetch(url);
    };
  });

  await page.route('**/api/v1/nodes', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: [{ id: 'node-1', display_name: 'Tokyo', hostname: 'tokyo', ip: '1.1.1.1', online_state: 'online', version: 'v2', max_groups: 50, last_heartbeat_at: '2026-04-01T00:00:00Z' }],
      }),
    });
  });

  await page.route('**/api/v1/node-enrollments', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: [] }) });
  });

  await page.goto('/nodes');
  await page.getByTestId('open-node-logs-node-1').click();
  await page.getByTestId('start-node-logs').click();

  await page.evaluate(() => {
    window.__pushStreamChunk('event: status\ndata: {"type":"status","content":"live_connected","timestamp":"2026-04-01T12:00:00Z"}\n\n');
    window.__pushStreamChunk('event: history\ndata: {"type":"history","content":"old line","timestamp":"2026-04-01T11:59:00Z"}\n\n');
    window.__pushStreamChunk('event: live\ndata: {"type":"live","content":"new line","timestamp":"2026-04-01T12:00:01Z"}\n\n');
  });

  await expect(page.getByTestId('node-logs-status')).toContainText('实时流已连接');
  await expect(page.getByTestId('node-logs-history')).toContainText('old line');
  await expect(page.getByTestId('node-logs-live')).toContainText('new line');
});
```

- [ ] **Step 2: Run the Playwright test to verify it fails**

Run:

```bash
cd web && npm run test:e2e -- tests/node-logs.spec.js --project=chromium
```

Expected: FAIL because the Nodes page has no log button, dialog, or authenticated stream wiring.

- [ ] **Step 3: Implement the log dialog and stream parsing in `NodeList.vue`**

Add log controls to the operations column:

```vue
<el-button size="small" data-testid="open-node-logs-{{ row.id }}" @click="openLogDialog(row)">日志</el-button>
```

Use a bound attribute in Vue instead:

```vue
<el-button size="small" :data-testid="`open-node-logs-${row.id}`" @click="openLogDialog(row)">日志</el-button>
```

Add the dialog to `web/src/views/NodeList.vue`:

```vue
<el-dialog v-model="logVisible" title="节点日志" width="80%">
  <el-form :inline="true">
    <el-form-item label="日志源">
      <el-select v-model="logForm.sourceType">
        <el-option label="节点服务日志" value="node_service" />
        <el-option label="容器日志" value="container" />
      </el-select>
    </el-form-item>
    <el-form-item v-if="logForm.sourceType === 'container'" label="容器名">
      <el-input v-model="logForm.containerName" />
    </el-form-item>
  </el-form>

  <el-alert :title="logStatus" :closable="false" data-testid="node-logs-status" />

  <div class="log-grid">
    <div>
      <h3>实时日志</h3>
      <pre data-testid="node-logs-live" class="log-pane">{{ liveLogs.join('\n') }}</pre>
    </div>
    <div>
      <h3>最近 200 行</h3>
      <pre data-testid="node-logs-history" class="log-pane">{{ historyLogs.join('\n') }}</pre>
    </div>
  </div>

  <template #footer>
    <el-button @click="clearLogs">清空</el-button>
    <el-button @click="stopLogs">停止</el-button>
    <el-button data-testid="start-node-logs" type="primary" @click="startLogs">开始</el-button>
    <el-button @click="restartLogs">重新连接</el-button>
  </template>
</el-dialog>
```

Add the script logic:

```js
const logVisible = ref(false)
const logNode = ref(null)
const logStatus = ref('未连接')
const liveLogs = ref([])
const historyLogs = ref([])
const logForm = ref({ sourceType: 'node_service', containerName: '' })
let logAbortController = null

const buildLogUrl = () => {
  const params = new URLSearchParams({ source_type: logForm.value.sourceType })
  if (logForm.value.sourceType === 'container') {
    params.set('container_name', logForm.value.containerName)
  }
  return `/api/v1/nodes/${logNode.value.id}/logs/stream?${params.toString()}`
}

const openLogDialog = (row) => {
  logNode.value = row
  logStatus.value = '未连接'
  liveLogs.value = []
  historyLogs.value = []
  logForm.value = { sourceType: 'node_service', containerName: '' }
  logVisible.value = true
}

const parseEventStream = async (response) => {
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })

    let splitIndex
    while ((splitIndex = buffer.indexOf('\n\n')) !== -1) {
      const frame = buffer.slice(0, splitIndex)
      buffer = buffer.slice(splitIndex + 2)
      const lines = frame.split('\n')
      const event = lines.find(line => line.startsWith('event:'))?.slice(6).trim()
      const dataLine = lines.find(line => line.startsWith('data:'))?.slice(5).trim()
      if (!event || !dataLine) continue
      const data = JSON.parse(dataLine)
      if (event === 'status') {
        logStatus.value = data.content === 'live_connected' ? '实时流已连接' : data.content
      } else if (event === 'history') {
        historyLogs.value.push(data.content)
      } else if (event === 'live') {
        liveLogs.value.push(data.content)
      } else if (event === 'error') {
        logStatus.value = data.content
      }
    }
  }
}

const stopLogs = () => {
  if (logAbortController) {
    logAbortController.abort()
    logAbortController = null
  }
  logStatus.value = '日志流已停止'
}

const clearLogs = () => {
  liveLogs.value = []
  historyLogs.value = []
}

const startLogs = () => {
  if (logForm.value.sourceType === 'container' && !logForm.value.containerName) {
    ElMessage.error('请输入容器名')
    return
  }

  stopLogs()
  logStatus.value = '正在连接实时日志...'
  logAbortController = new AbortController()
  fetch(buildLogUrl(), {
    headers: {
      Authorization: `Basic ${localStorage.getItem('auth_token')}`,
      Accept: 'text/event-stream',
    },
    signal: logAbortController.signal,
  })
    .then(async (response) => {
      if (!response.ok) {
        const text = await response.text()
        throw new Error(text || '日志流建立失败')
      }
      await parseEventStream(response)
      logStatus.value = '日志流已断开'
    })
    .catch((error) => {
      if (error.name !== 'AbortError') {
        logStatus.value = error.message || '日志流已断开'
      }
    })
}

const restartLogs = () => {
  startLogs()
}
```

- [ ] **Step 4: Run the Playwright test to verify it passes**

Run:

```bash
cd web && npm run test:e2e -- tests/node-logs.spec.js --project=chromium
```

Expected: PASS with one scenario proving the dialog opens and renders separate live/history panes from mocked stream events.

- [ ] **Step 5: Commit the Nodes-page log UI**

```bash
git add web/src/views/NodeList.vue web/tests/node-logs.spec.js
git commit -m "feat: add remote node logs UI"
```

## Task 5: Full Verification and Smoke Checks

**Files:**
- Modify: `api/proto/platform.proto`
- Modify: `api/pb/platform.pb.go`
- Modify: `api/pb/platform_grpc.pb.go`
- Modify: `cmd/manager/grpc_server.go`
- Modify: `cmd/manager/http_server.go`
- Modify: `cmd/manager/http_node_logs.go`
- Modify: `cmd/manager/node_log_hub.go`
- Modify: `cmd/node/log_stream.go`
- Modify: `cmd/node/main.go`
- Modify: `web/src/views/NodeList.vue`
- Modify: `web/tests/node-logs.spec.js`

- [ ] **Step 1: Format all touched Go files**

Run:

```bash
gofmt -w cmd/manager/node_log_hub.go cmd/manager/node_log_hub_test.go cmd/manager/http_node_logs.go cmd/manager/http_node_logs_test.go cmd/manager/grpc_server.go cmd/node/log_stream.go cmd/node/log_stream_test.go cmd/node/main.go
```

Expected: no output, files rewritten in place.

- [ ] **Step 2: Run the manager and node targeted Go tests**

Run:

```bash
go test ./cmd/manager ./cmd/node -v
```

Expected: PASS, including the new log-hub, stream validation, and node log-command tests.

- [ ] **Step 3: Build the frontend and rerun both targeted E2E tests**

Run:

```bash
cd web && npm run build && npm run test:e2e -- tests/node-enrollment.spec.js tests/node-logs.spec.js --project=chromium
```

Expected: Vite build succeeds and both Node-page E2E tests pass.

- [ ] **Step 4: Smoke-check the new stream endpoint manually**

Run:

```bash
curl -N -u admin:admin "http://127.0.0.1:8002/api/v1/nodes/<node-id>/logs/stream?source_type=node_service"
```

Expected: stream emits `event: status`, then `event: history`, then `event: live` lines if the target node is online and has an active `LogStream`.

- [ ] **Step 5: Commit the verification pass**

```bash
git add api/proto/platform.proto api/pb/platform.pb.go api/pb/platform_grpc.pb.go cmd/manager/node_log_hub.go cmd/manager/node_log_hub_test.go cmd/manager/http_node_logs.go cmd/manager/http_node_logs_test.go cmd/manager/grpc_server.go cmd/manager/http_server.go cmd/node/log_stream.go cmd/node/log_stream_test.go cmd/node/main.go web/src/views/NodeList.vue web/tests/node-logs.spec.js
git commit -m "test: verify remote node log streaming"
```
