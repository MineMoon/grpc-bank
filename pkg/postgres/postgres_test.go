package postgres

import (
	"context"
	"strings"
	"testing"
)

func TestNewPGXPoolRejectsInvalidConfig(t *testing.T) {
	_, err := NewPGXPool(context.Background(), "postgres://user:pass@127.0.0.1:not-a-port/db")
	if err == nil {
		t.Fatal("expected invalid connection string to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "config parsing error") {
		t.Fatalf("expected config parsing error, got %v", err)
	}
}
