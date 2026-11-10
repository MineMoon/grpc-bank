package config

import (
	"strings"
	"testing"
)

func setPostgres(t *testing.T) {
	t.Helper()
	t.Setenv("POSTGRES_USER", "bank")
	t.Setenv("POSTGRES_PASSWORD", "secret")
	t.Setenv("POSTGRES_HOST", "db.example")
	t.Setenv("POSTGRES_PORT", "6543")
	t.Setenv("POSTGRES_SSLMODE", "require")
	t.Setenv("ACCOUNTS_DB_NAME", "accounts_db")
}

func TestLoad(t *testing.T) {
	t.Run("builds the database url", func(t *testing.T) {
		setPostgres(t)
		t.Setenv("ACCOUNT_SERVICE_GRPC_PORT", "60051")
		t.Setenv("ACCOUNT_SERVICE_DB_URL", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.GRPCPort != "60051" {
			t.Fatalf("expected port 60051, got %q", cfg.GRPCPort)
		}
		want := "postgres://bank:secret@db.example:6543/accounts_db?sslmode=require"
		if cfg.DBURL != want {
			t.Fatalf("expected %q, got %q", want, cfg.DBURL)
		}
	})

	t.Run("explicit url wins", func(t *testing.T) {
		setPostgres(t)
		t.Setenv("ACCOUNT_SERVICE_GRPC_PORT", "60051")
		t.Setenv("ACCOUNT_SERVICE_DB_URL", "postgres://user:pass@db:5432/custom")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.DBURL != "postgres://user:pass@db:5432/custom" {
			t.Fatalf("unexpected db url %q", cfg.DBURL)
		}
	})

	t.Run("reports missing variables", func(t *testing.T) {
		setPostgres(t)
		t.Setenv("ACCOUNT_SERVICE_GRPC_PORT", "")
		t.Setenv("ACCOUNT_SERVICE_DB_URL", "")

		_, err := Load()
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "ACCOUNT_SERVICE_GRPC_PORT") {
			t.Fatalf("error %q does not name the missing variable", err)
		}
	})
}
