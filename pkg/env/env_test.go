package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	return path
}

func TestLoad(t *testing.T) {
	t.Run("reads keys, quotes and comments", func(t *testing.T) {
		t.Setenv("LOAD_PLAIN", "")
		t.Setenv("LOAD_EXPORTED", "")
		t.Setenv("LOAD_DOUBLE", "")
		t.Setenv("LOAD_SINGLE", "")

		path := writeEnvFile(t, `
# comment
LOAD_PLAIN=value
export LOAD_EXPORTED=exported
LOAD_DOUBLE="quoted value"
LOAD_SINGLE='single value'

not a pair
`)
		if err := Load(path); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := map[string]string{
			"LOAD_PLAIN":    "value",
			"LOAD_EXPORTED": "exported",
			"LOAD_DOUBLE":   "quoted value",
			"LOAD_SINGLE":   "single value",
		}
		for key, value := range want {
			if got := os.Getenv(key); got != value {
				t.Fatalf("%s: expected %q, got %q", key, value, got)
			}
		}
	})

	t.Run("keeps values already in the environment", func(t *testing.T) {
		t.Setenv("LOAD_PRESET", "from-env")
		path := writeEnvFile(t, "LOAD_PRESET=from-file\n")

		if err := Load(path); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := os.Getenv("LOAD_PRESET"); got != "from-env" {
			t.Fatalf("expected environment value, got %q", got)
		}
	})

	t.Run("fills empty variables from the file", func(t *testing.T) {
		t.Setenv("LOAD_EMPTY", "")
		path := writeEnvFile(t, "LOAD_EMPTY=from-file\n")

		if err := Load(path); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := os.Getenv("LOAD_EMPTY"); got != "from-file" {
			t.Fatalf("expected file value, got %q", got)
		}
	})

	t.Run("missing file is not an error", func(t *testing.T) {
		if err := Load(filepath.Join(t.TempDir(), "absent")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestFile(t *testing.T) {
	t.Run("defaults to .env", func(t *testing.T) {
		t.Setenv("ENV_FILE", "")
		if got := File(); got != DefaultFile {
			t.Fatalf("expected %q, got %q", DefaultFile, got)
		}
	})

	t.Run("honours ENV_FILE", func(t *testing.T) {
		t.Setenv("ENV_FILE", "config/dev.env")
		if got := File(); got != "config/dev.env" {
			t.Fatalf("unexpected path %q", got)
		}
	})
}

func TestRequired(t *testing.T) {
	t.Run("passes when all keys are set", func(t *testing.T) {
		t.Setenv("REQ_A", "a")
		t.Setenv("REQ_B", "b")

		if err := Required("REQ_A", "REQ_B"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("names every missing key", func(t *testing.T) {
		t.Setenv("REQ_A", "")
		t.Setenv("REQ_B", "")

		err := Required("REQ_A", "REQ_B")
		if err == nil {
			t.Fatal("expected an error")
		}
		for _, key := range []string{"REQ_A", "REQ_B"} {
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("error %q does not name %s", err, key)
			}
		}
	})
}

func TestDatabaseURL(t *testing.T) {
	setPostgres := func(t *testing.T) {
		t.Helper()
		t.Setenv("POSTGRES_USER", "bank")
		t.Setenv("POSTGRES_PASSWORD", "p@ss word")
		t.Setenv("POSTGRES_HOST", "db.example")
		t.Setenv("POSTGRES_PORT", "6543")
		t.Setenv("POSTGRES_SSLMODE", "require")
		t.Setenv("TEST_DB_NAME", "test_db")
	}

	t.Run("builds a url from parts", func(t *testing.T) {
		setPostgres(t)
		t.Setenv("TEST_DB_URL", "")

		got, err := DatabaseURL("TEST_DB_URL", "TEST_DB_NAME")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "postgres://bank:p%40ss%20word@db.example:6543/test_db?sslmode=require"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("explicit override wins", func(t *testing.T) {
		setPostgres(t)
		t.Setenv("TEST_DB_URL", "postgres://user:pass@custom:5432/custom")

		got, err := DatabaseURL("TEST_DB_URL", "TEST_DB_NAME")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "postgres://user:pass@custom:5432/custom" {
			t.Fatalf("unexpected url %q", got)
		}
	})

	t.Run("reports missing parts", func(t *testing.T) {
		setPostgres(t)
		t.Setenv("TEST_DB_URL", "")
		t.Setenv("POSTGRES_PORT", "")

		if _, err := DatabaseURL("TEST_DB_URL", "TEST_DB_NAME"); err == nil {
			t.Fatal("expected an error")
		} else if !strings.Contains(err.Error(), "POSTGRES_PORT") {
			t.Fatalf("error %q does not name POSTGRES_PORT", err)
		}
	})
}
