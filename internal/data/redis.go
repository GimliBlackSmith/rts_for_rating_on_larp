package data

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

type RedisClient struct {
	address  string
	password string
	db       string
}

func NewRedisClient(ctx context.Context, addr string) (*RedisClient, error) {
	if addr == "" {
		addr = defaultRedisURL()
	}

	parsed, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	password, _ := parsed.User.Password()
	db := strings.TrimPrefix(parsed.Path, "/")
	if db == "" {
		db = "0"
	}

	host := parsed.Host
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "6379")
	}

	client := &RedisClient{
		address:  host,
		password: password,
		db:       db,
	}

	if err := client.Ping(ctx); err != nil {
		return nil, err
	}

	return client, nil
}

func (c *RedisClient) Close() error {
	return nil
}

func (c *RedisClient) Ping(ctx context.Context) error {
	if c == nil {
		return errors.New("redis client is nil")
	}

	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", c.address)
	if err != nil {
		return fmt.Errorf("dial redis: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	if c.password != "" {
		if err := sendRESP(writer, []string{"AUTH", c.password}); err != nil {
			return err
		}
		if err := expectSimpleString(reader, "OK"); err != nil {
			return fmt.Errorf("redis auth failed: %w", err)
		}
	}

	if c.db != "0" {
		if err := sendRESP(writer, []string{"SELECT", c.db}); err != nil {
			return err
		}
		if err := expectSimpleString(reader, "OK"); err != nil {
			return fmt.Errorf("redis select failed: %w", err)
		}
	}

	if err := sendRESP(writer, []string{"PING"}); err != nil {
		return err
	}

	if err := expectSimpleString(reader, "PONG"); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}

	return nil
}

func sendRESP(w *bufio.Writer, args []string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return w.Flush()
}

func expectSimpleString(r *bufio.Reader, want string) error {
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "+") {
		return fmt.Errorf("unexpected redis response: %s", line)
	}
	if got := strings.TrimPrefix(line, "+"); got != want {
		return fmt.Errorf("unexpected redis response: %s", got)
	}
	return nil
}

func defaultRedisURL() string {
	host := strings.TrimSpace(os.Getenv("REDIS_HOST"))
	if host == "" {
		host = "redis"
	}
	port := strings.TrimSpace(os.Getenv("REDIS_PORT"))
	if port == "" {
		port = "6379"
	}
	db := strings.TrimSpace(os.Getenv("REDIS_DB"))
	if db == "" {
		db = "0"
	}
	return fmt.Sprintf("redis://%s:%s/%s", host, port, db)
}
