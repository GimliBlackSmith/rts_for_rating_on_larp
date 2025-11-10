package data

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

type PostgresPool struct {
	address string
}

func NewPostgresPool(ctx context.Context, dsn string) (*PostgresPool, error) {
	if dsn == "" {
		dsn = defaultPostgresURL()
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}

	host := u.Host
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "5432")
	}

	pool := &PostgresPool{address: host}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	return pool, nil
}

func (p *PostgresPool) Close() error {
	return nil
}

func (p *PostgresPool) Ping(ctx context.Context) error {
	if p == nil {
		return errors.New("postgres pool is nil")
	}

	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", p.address)
	if err != nil {
		return fmt.Errorf("dial postgres: %w", err)
	}
	_ = conn.Close()
	return nil
}

func defaultPostgresURL() string {
	host := getenvDefault("POSTGRES_HOST", "postgres")
	port := getenvDefault("POSTGRES_PORT", "5432")
	user := getenvDefault("POSTGRES_USER", "rts_user")
	password := getenvDefault("POSTGRES_PASSWORD", "rts_password")
	db := getenvDefault("POSTGRES_DB", "rts_db")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, db)
}

func getenvDefault(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}
