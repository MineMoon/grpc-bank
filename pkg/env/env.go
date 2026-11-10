// Package env loads service configuration from a .env file and the process
// environment.
package env

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// DefaultFile is read when ENV_FILE is not set.
const DefaultFile = ".env"

// File returns the env file to read: ENV_FILE when set, DefaultFile otherwise.
func File() string {
	if path := os.Getenv("ENV_FILE"); path != "" {
		return path
	}
	return DefaultFile
}

// Load reads KEY=VALUE lines from path into the process environment. Blank
// lines and lines starting with # are ignored. Non-empty variables already
// present in the environment are kept, so exported values override the file. A
// missing file is not an error: services can run on the exported environment
// alone.
func Load(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		key, value, ok := parseLine(scanner.Text())
		if !ok {
			continue
		}
		if os.Getenv(key) != "" {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("%s:%d: %w", path, line, err)
		}
	}

	return scanner.Err()
}

func parseLine(raw string) (key, value string, ok bool) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}

	line = strings.TrimPrefix(line, "export ")
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}

	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		value = value[1 : len(value)-1]
	}
	if key == "" {
		return "", "", false
	}

	return key, value, true
}

// Required checks that every key is set and non-empty, and names all the
// missing variables in the returned error.
func Required(keys ...string) error {
	var missing []string
	for _, key := range keys {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing environment variables: %s (copy .env.example to .env)", strings.Join(missing, ", "))
	}

	return nil
}

// DatabaseURL returns the connection string for a database. An explicit
// overrideKey value wins; otherwise the URL is built from POSTGRES_USER,
// POSTGRES_PASSWORD, POSTGRES_HOST, POSTGRES_PORT, POSTGRES_SSLMODE and the
// database name stored under databaseKey.
func DatabaseURL(overrideKey, databaseKey string) (string, error) {
	if override := os.Getenv(overrideKey); override != "" {
		return override, nil
	}

	if err := Required(
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_HOST",
		"POSTGRES_PORT",
		"POSTGRES_SSLMODE",
		databaseKey,
	); err != nil {
		return "", err
	}

	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD")),
		Host:     net.JoinHostPort(os.Getenv("POSTGRES_HOST"), os.Getenv("POSTGRES_PORT")),
		Path:     "/" + os.Getenv(databaseKey),
		RawQuery: url.Values{"sslmode": {os.Getenv("POSTGRES_SSLMODE")}}.Encode(),
	}

	return dsn.String(), nil
}
