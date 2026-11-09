package config

import "testing"

func TestLoad(t *testing.T) {
	t.Run("uses defaults", func(t *testing.T) {
		t.Setenv("ACCOUNT_SERVICE_GRPC_PORT", "")
		t.Setenv("ACCOUNT_SERVICE_DB_URL", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.GRPCPort != "50051" {
			t.Fatalf("expected port 50051, got %q", cfg.GRPCPort)
		}
		if cfg.DBURL != "postgres://postgres:postgresadmin@127.0.0.1:5432/accounts_db?sslmode=disable" {
			t.Fatalf("unexpected db url %q", cfg.DBURL)
		}
	})

	t.Run("reads environment", func(t *testing.T) {
		t.Setenv("ACCOUNT_SERVICE_GRPC_PORT", "60051")
		t.Setenv("ACCOUNT_SERVICE_DB_URL", "postgres://user:pass@db:5432/custom")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.GRPCPort != "60051" {
			t.Fatalf("expected port 60051, got %q", cfg.GRPCPort)
		}
		if cfg.DBURL != "postgres://user:pass@db:5432/custom" {
			t.Fatalf("unexpected db url %q", cfg.DBURL)
		}
	})
}
