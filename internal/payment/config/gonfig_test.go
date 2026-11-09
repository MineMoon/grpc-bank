package config

import "testing"

func TestLoad(t *testing.T) {
	t.Run("uses defaults", func(t *testing.T) {
		t.Setenv("PAYMENT_SERVICE_GRPC_PORT", "")
		t.Setenv("PAYMENT_SERVICE_DB_URL", "")
		t.Setenv("ACCOUNT_SERVICE_ADDR", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.GRPCPort != "50052" {
			t.Fatalf("expected port 50052, got %q", cfg.GRPCPort)
		}
		if cfg.DBURL != "postgres://postgres:postgresadmin@127.0.0.1:5432/payments_db?sslmode=disable" {
			t.Fatalf("unexpected db url %q", cfg.DBURL)
		}
		if cfg.AccountServiceAddr != "127.0.0.1:50051" {
			t.Fatalf("unexpected account service address %q", cfg.AccountServiceAddr)
		}
	})

	t.Run("reads environment", func(t *testing.T) {
		t.Setenv("PAYMENT_SERVICE_GRPC_PORT", "60052")
		t.Setenv("PAYMENT_SERVICE_DB_URL", "postgres://user:pass@db:5432/custom")
		t.Setenv("ACCOUNT_SERVICE_ADDR", "account:60051")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.GRPCPort != "60052" {
			t.Fatalf("expected port 60052, got %q", cfg.GRPCPort)
		}
		if cfg.DBURL != "postgres://user:pass@db:5432/custom" {
			t.Fatalf("unexpected db url %q", cfg.DBURL)
		}
		if cfg.AccountServiceAddr != "account:60051" {
			t.Fatalf("unexpected account service address %q", cfg.AccountServiceAddr)
		}
	})
}
