package main

import (
	"strings"
	"testing"
)

func TestBuildBrowserRunArgsUsesDedicatedTunnelNetwork(t *testing.T) {
	payload := BrowserStartPayload{
		ContainerName: "browser-a",
		BrowserImage:  "kasmweb/chromium:1.16.1",
		VNCPassword:   "secret",
	}
	args := buildBrowserRunArgs(payload, "browser-tunnel-a")

	found := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--network" && args[i+1] == "container:browser-tunnel-a" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dedicated tunnel network args, got %v", args)
	}

	chromeArgsFound := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && strings.HasPrefix(args[i+1], "CHROME_ARGS=") {
			chromeArgsFound = true
			value := args[i+1]
			for _, want := range []string{"--disable-gpu", "--disable-gpu-compositing", "--disable-accelerated-2d-canvas", "--use-gl=swiftshader", "--disable-dev-shm-usage", "--no-sandbox"} {
				if !strings.Contains(value, want) {
					t.Fatalf("missing %s in CHROME_ARGS: %s", want, value)
				}
			}
			break
		}
	}
	if !chromeArgsFound {
		t.Fatalf("expected CHROME_ARGS env, got %v", args)
	}

	xdgRuntimeFound := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && args[i+1] == "XDG_RUNTIME_DIR=/tmp/runtime-kasm" {
			xdgRuntimeFound = true
			break
		}
	}
	if !xdgRuntimeFound {
		t.Fatalf("expected XDG_RUNTIME_DIR env, got %v", args)
	}
}

func TestBuildBrowserRunArgsWithoutProxyKeepsDefaultNetwork(t *testing.T) {
	payload := BrowserStartPayload{
		ContainerName: "browser-a",
		BrowserImage:  "kasmweb/chromium:1.16.1",
		VNCPassword:   "secret",
	}
	args := buildBrowserRunArgs(payload, "")

	for _, arg := range args {
		if arg == "--network" {
			t.Fatalf("did not expect --network when no dedicated tunnel is used: %v", args)
		}
	}
}
