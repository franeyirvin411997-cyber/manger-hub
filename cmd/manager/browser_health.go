package main

import (
	"net"
	"time"

	"multi_node_platform/pkg/models"
	"multi_node_platform/pkg/workers"
)

func evaluateBrowserHealth(browser models.BrowserInstance, node models.Node, now time.Time, dialFn func(network, addr string, timeout time.Duration) (net.Conn, error)) (string, string) {
	return workers.EvaluateBrowserHealth(browser, node, now, dialFn)
}
