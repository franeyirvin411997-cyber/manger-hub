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
