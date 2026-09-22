package main

import (
	"testing"
)

func TestParseFlags(t *testing.T) {
	configPath, certificatePath, privateKeyPath, err := parseFlags([]string{
		"--config", "gateway.yaml",
		"--tls-cert", "server.crt",
		"--tls-key", "server.key",
	})
	if err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}
	if configPath != "gateway.yaml" || certificatePath != "server.crt" || privateKeyPath != "server.key" {
		t.Fatalf("parseFlags() = %q, %q, %q", configPath, certificatePath, privateKeyPath)
	}

	if _, _, _, err := parseFlags([]string{"--config", "gateway.yaml"}); err == nil {
		t.Fatal("parseFlags() missing TLS arguments error = nil")
	}
	if _, _, _, err := parseFlags([]string{
		"--config", "gateway.yaml",
		"--tls-cert", "server.crt",
		"--tls-key", "server.key",
		"extra",
	}); err == nil {
		t.Fatal("parseFlags() extra argument error = nil")
	}
}
