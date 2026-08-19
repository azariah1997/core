package health

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

type Check struct{ Name, Address string }
type Result struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Checks probes a service's dependencies and returns their results.
type Checks func(ctx context.Context) []Result

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

// Live reports that the process is alive. It never depends on external
// systems, so orchestrators use it to decide whether to restart the
// container. version is the real git commit SHA this process was built
// from (config.Config.Version) - Phase 29's control-plane "versions"
// dimension reads it from here rather than a separate endpoint, since
// every service already exposes this uniformly.
func Live(service, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok", "service": service, "version": version})
	}
}

// Ready reports whether the dependencies required to serve traffic are
// available. Orchestrators use it to gate traffic to the instance.
func Ready(checks Checks) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, ok := run(r, checks)
		status := http.StatusOK
		if !ok {
			status = http.StatusServiceUnavailable
		}
		httpx.JSON(w, status, map[string]any{"ready": ok, "dependencies": results})
	}
}

// Health returns an aggregated operational view combining liveness and
// dependency status, for dashboards and manual inspection. It always
// returns 200 so it never triggers orchestrator action on its own.
func Health(service, version string, checks Checks) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, ok := run(r, checks)
		httpx.JSON(w, http.StatusOK, map[string]any{
			"status": "ok", "service": service, "version": version, "ready": ok, "dependencies": results,
		})
	}
}

func run(r *http.Request, checks Checks) ([]Result, bool) {
	if checks == nil {
		return []Result{}, true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	results := checks(ctx)
	ok := true
	for _, c := range results {
		if !c.OK {
			ok = false
		}
	}
	return results, ok
}
