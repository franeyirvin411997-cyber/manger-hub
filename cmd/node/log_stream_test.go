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
