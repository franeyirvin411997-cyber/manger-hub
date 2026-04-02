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

type managedNodeLogStream struct {
	stream pb.NodeService_LogStreamServer
	mu     sync.Mutex
}

func (s *managedNodeLogStream) Send(msg *pb.LogEnvelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stream.Send(msg)
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

var managerNodeLogHub = newNodeLogHub()

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
