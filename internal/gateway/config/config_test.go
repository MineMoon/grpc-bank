package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Run("reads environment", func(t *testing.T) {
		t.Setenv("GATEWAY_HTTP_PORT", "9080")
		t.Setenv("ACCOUNT_SERVICE_ADDR", "account:60051")
		t.Setenv("PAYMENT_SERVICE_ADDR", "payment:60052")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.HTTPPort != "9080" {
			t.Fatalf("expected port 9080, got %q", cfg.HTTPPort)
		}
		if cfg.AccountServiceAddr != "account:60051" {
			t.Fatalf("unexpected account service address %q", cfg.AccountServiceAddr)
		}
		if cfg.PaymentServiceAddr != "payment:60052" {
			t.Fatalf("unexpected payment service address %q", cfg.PaymentServiceAddr)
		}
	})

	t.Run("reports missing variables", func(t *testing.T) {
		t.Setenv("GATEWAY_HTTP_PORT", "9080")
		t.Setenv("ACCOUNT_SERVICE_ADDR", "account:60051")
		t.Setenv("PAYMENT_SERVICE_ADDR", "")

		_, err := Load()
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "PAYMENT_SERVICE_ADDR") {
			t.Fatalf("error %q does not name the missing variable", err)
		}
	})
}
