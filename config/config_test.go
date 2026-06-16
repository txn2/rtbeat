package config

import (
	"testing"

	"github.com/elastic/beats/v7/libbeat/common"
)

func TestDefaultConfig(t *testing.T) {
	if DefaultConfig.Port != "8081" {
		t.Errorf("default Port = %q, want %q", DefaultConfig.Port, "8081")
	}
	if DefaultConfig.Timeout != 5 {
		t.Errorf("default Timeout = %d, want 5", DefaultConfig.Timeout)
	}
}

func TestUnpackOverrides(t *testing.T) {
	raw, err := common.NewConfigFrom(map[string]interface{}{
		"port":    "9090",
		"timeout": 30,
	})
	if err != nil {
		t.Fatalf("NewConfigFrom: %v", err)
	}

	c := DefaultConfig
	if err := raw.Unpack(&c); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	if c.Port != "9090" {
		t.Errorf("Port = %q, want %q", c.Port, "9090")
	}
	if c.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", c.Timeout)
	}
}

func TestUnpackEmptyKeepsDefaults(t *testing.T) {
	raw, err := common.NewConfigFrom(map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewConfigFrom: %v", err)
	}

	c := DefaultConfig
	if err := raw.Unpack(&c); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	if c.Port != "8081" || c.Timeout != 5 {
		t.Errorf("defaults not preserved after empty unpack: %+v", c)
	}
}
