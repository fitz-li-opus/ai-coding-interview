package config

import (
	"strings"
	"testing"
	"time"
)

const validConfig = `
server:
  address: ":8443"
upstreams:
  - id: users
    endpoints: ["http://user-service:8080"]
    health:
      path: /health
      interval: 5s
      timeout: 1s
routes:
  - id: users
    hosts: ["Users.Internal.Test."]
    path_prefix: /api/users
    upstream: users
    timeout: 3s
    rate_limit:
      requests_per_second: 10
      burst: 20
`

func TestDecodeAppliesDefaultsAndNormalizesHosts(t *testing.T) {
	cfg, err := Decode(strings.NewReader(validConfig))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if cfg.Server.MaxBodyBytes != 1024*1024 {
		t.Fatalf("MaxBodyBytes = %d", cfg.Server.MaxBodyBytes)
	}
	if cfg.Server.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %v", cfg.Server.ReadHeaderTimeout)
	}
	if cfg.Routes[0].Hosts[0] != "users.internal.test" {
		t.Fatalf("normalized host = %q", cfg.Routes[0].Hosts[0])
	}
}

func TestExampleConfiguration(t *testing.T) {
	if _, err := Load("../../config.example.yaml"); err != nil {
		t.Fatalf("Load(config.example.yaml) error = %v", err)
	}
}

func TestDecodeRejectsInvalidConfiguration(t *testing.T) {
	tests := map[string]string{
		"unknown field": strings.Replace(validConfig, "  address: \":8443\"", "  address: \":8443\"\n  unexpected: true", 1),
		"duplicate route": validConfig + `
  - id: users-copy
    hosts: ["users.internal.test"]
    path_prefix: /api/users
    upstream: users
    timeout: 3s
`,
		"unknown upstream":   strings.Replace(validConfig, "upstream: users", "upstream: missing", 1),
		"invalid endpoint":   strings.Replace(validConfig, "http://user-service:8080", "file:///etc/passwd", 1),
		"multiple documents": validConfig + "\n---\n{}\n",
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(strings.NewReader(input)); err == nil {
				t.Fatal("Decode() error = nil")
			}
		})
	}
}
