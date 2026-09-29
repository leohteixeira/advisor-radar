package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestComposeContract(t *testing.T) {
	t.Parallel()

	compose, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatalf("read compose.yaml: %v", err)
	}
	initSQL, err := os.ReadFile("deploy/postgres/init.sql")
	if err != nil {
		t.Fatalf("read init.sql: %v", err)
	}

	text := string(compose)
	if !strings.Contains(text, "name: advisor-radar") {
		t.Fatal("compose.yaml missing project name advisor-radar")
	}

	wantPorts := []string{"5435:5432", "5673:5672", "15673:15672", "9201:9200", "3410:3000", "4417:4317", "4418:4318"}
	for _, port := range wantPorts {
		entry := regexp.MustCompile(`(?m)^\s+-\s+"` + regexp.QuoteMeta(port) + `"\s*$`)
		if !entry.MatchString(text) {
			t.Fatalf("compose.yaml missing uncommented published port list entry %q", port)
		}
	}

	if !strings.Contains(text, "./deploy/postgres/init.sql:/docker-entrypoint-initdb.d/init.sql") {
		t.Fatal("compose.yaml missing init.sql volume mount")
	}
	if !strings.Contains(text, "discovery.type: single-node") {
		t.Fatal("compose.yaml missing discovery.type single-node")
	}
	if !strings.Contains(text, "ES_JAVA_OPTS:") || !strings.Contains(text, "512m") {
		t.Fatal("compose.yaml ES_JAVA_OPTS must contain 512m")
	}

	// Application listen ports must not be published by this story.
	appPorts := regexp.MustCompile(`(?m)^\s+-\s+"(3400|8400):\d+"\s*$`)
	if appPorts.MatchString(text) {
		t.Fatal("compose.yaml must not publish application ports 3400 or 8400")
	}

	// 3000, 4317, and 4318 on the host belong to Cybersecurity and to any
	// collector outside this repository.
	foreignPorts := regexp.MustCompile(`(?m)^\s+-\s+"(3000|4317|4318):\d+"\s*$`)
	if foreignPorts.MatchString(text) {
		t.Fatal("compose.yaml must not publish host ports 3000, 4317, or 4318")
	}
	if !regexp.MustCompile(`(?m)^\s+image:\s+grafana/otel-lgtm:\d+\.\d+\.\d+\s*$`).MatchString(text) {
		t.Fatal("compose.yaml must pin grafana/otel-lgtm to a release tag")
	}

	if strings.Contains(strings.ToLower(text), "mysql") {
		t.Fatal("compose.yaml must not define MySQL")
	}

	initText := string(initSQL)
	for _, db := range []string{"account_sim", "advisory", "triage", "cases"} {
		if !strings.Contains(initText, "CREATE DATABASE "+db) {
			t.Fatalf("init.sql missing database %q", db)
		}
	}
}
