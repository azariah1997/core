package health

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"
)

type Check struct{ Name, Address string }
type Result struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func TCP(ctx context.Context, c Check) Result {
	address := normalize(c.Address)
	d := net.Dialer{Timeout: 750 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return Result{Name: c.Name, OK: false, Error: err.Error()}
	}
	_ = conn.Close()
	return Result{Name: c.Name, OK: true}
}

func normalize(s string) string {
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return s
}
