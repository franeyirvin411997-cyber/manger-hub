package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	pb "multi_node_platform/api/pb"
)

type logEnvelopeSender interface {
	Send(*pb.LogEnvelope) error
}

type lockedLogStream struct {
	stream pb.NodeService_LogStreamClient
	mu     sync.Mutex
}

func (s *lockedLogStream) Send(msg *pb.LogEnvelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stream.Send(msg)
}

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

func sendLogEnvelope(stream logEnvelopeSender, msgType, subID, streamType, content string) error {
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
				log.Printf("日志流连接失败: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			locked := &lockedLogStream{stream: stream}
			if err := locked.Send(&pb.LogEnvelope{
				MessageType:   "stream_open",
				NodeId:        config.NodeID,
				Token:         token,
				TimestampUnix: time.Now().Unix(),
			}); err != nil {
				log.Printf("日志流握手失败: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}

			runLogStreamSession(stream, locked)
			time.Sleep(2 * time.Second)
		}
	}()
}

func runLogStreamSession(recvStream pb.NodeService_LogStreamClient, sendStream logEnvelopeSender) {
	var mu sync.Mutex
	cancelMap := map[string]context.CancelFunc{}

	for {
		msg, err := recvStream.Recv()
		if err != nil {
			mu.Lock()
			for _, cancel := range cancelMap {
				cancel()
			}
			mu.Unlock()
			return
		}

		switch msg.MessageType {
		case "subscribe_request":
			liveCmd, historyCmd, err := buildLogCommands(msg.SourceType, msg.ContainerName)
			if err != nil {
				_ = sendLogEnvelope(sendStream, "error", msg.SubscriptionId, "error", err.Error())
				continue
			}

			ctx, cancel := context.WithCancel(context.Background())
			mu.Lock()
			if oldCancel, ok := cancelMap[msg.SubscriptionId]; ok {
				oldCancel()
			}
			cancelMap[msg.SubscriptionId] = cancel
			mu.Unlock()

			go runLogSubscription(ctx, sendStream, msg.SubscriptionId, liveCmd, historyCmd)

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

func runLogSubscription(ctx context.Context, stream logEnvelopeSender, subscriptionID string, liveCmd, historyCmd []string) {
	_ = sendLogEnvelope(stream, "status", subscriptionID, "status", "live_connecting")
	if err := startLiveCommand(ctx, stream, subscriptionID, liveCmd); err != nil {
		_ = sendLogEnvelope(stream, "error", subscriptionID, "error", err.Error())
		return
	}

	_ = sendLogEnvelope(stream, "status", subscriptionID, "status", "live_connected")
	if err := emitHistory(stream, subscriptionID, historyCmd); err != nil {
		_ = sendLogEnvelope(stream, "error", subscriptionID, "error", err.Error())
	}
}

func emitHistory(stream logEnvelopeSender, subscriptionID string, args []string) error {
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
		if strings.TrimSpace(line) == "" {
			continue
		}
		_ = sendLogEnvelope(stream, "history", subscriptionID, "history", line)
	}
	return nil
}

func startLiveCommand(ctx context.Context, stream logEnvelopeSender, subscriptionID string, args []string) error {
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
		err := cmd.Wait()
		if err != nil && ctx.Err() == nil {
			_ = sendLogEnvelope(stream, "error", subscriptionID, "error", err.Error())
		}
		_ = sendLogEnvelope(stream, "status", subscriptionID, "status", "stream_closed")
	}()
	return nil
}

func scanLogPipe(stream logEnvelopeSender, subscriptionID string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		_ = sendLogEnvelope(stream, "live", subscriptionID, "live", scanner.Text())
	}
}
