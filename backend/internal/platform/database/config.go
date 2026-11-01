package database

import (
	"fmt"

	"github.com/jackc/pgx/v5"
)

func stdlibConfig(url string) (*pgx.ConnConfig, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("database: invalid URL: %w", err)
	}
	return cfg, nil
}
