package config

import (
	"os"

	"grpc-bank/pkg/env"
)

const (
	grpcPortKey = "ACCOUNT_SERVICE_GRPC_PORT"
	dbURLKey    = "ACCOUNT_SERVICE_DB_URL"
	databaseKey = "ACCOUNTS_DB_NAME"
)

type Config struct {
	GRPCPort string
	DBURL    string
}

func Load() (*Config, error) {
	if err := env.Required(grpcPortKey); err != nil {
		return nil, err
	}

	dbURL, err := env.DatabaseURL(dbURLKey, databaseKey)
	if err != nil {
		return nil, err
	}

	return &Config{
		GRPCPort: os.Getenv(grpcPortKey),
		DBURL:    dbURL,
	}, nil
}
