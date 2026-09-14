package api

import (
	"context"
	"net/http"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/marco/parkxchange/services/api/internal/web"
)

// readinessTimeout bounds the database check behind /readyz. A probe that
// hangs is worse than one that fails: the orchestrator learns nothing and the
// pod stays in rotation.
const readinessTimeout = 2 * time.Second

type healthResponse struct {
	Status string `json:"status"`
}

type readyResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

type versionResponse struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// handleHealthz is the liveness probe. It answers as long as the process can
// serve HTTP at all, and deliberately does not touch the database: a database
// blip must not cause every replica to be restarted at once.
func (a *API) handleHealthz(w http.ResponseWriter, _ *http.Request) error {
	return web.JSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// handleReadyz is the readiness probe. It reports whether this replica can do
// useful work, which requires a reachable database, so a replica that has lost
// the database is taken out of rotation without being killed.
func (a *API) handleReadyz(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	if err := a.health.Ping(ctx); err != nil {
		web.LoggerFrom(r.Context()).Warn("readiness check failed", "error", err)
		return web.JSON(w, http.StatusServiceUnavailable, readyResponse{
			Status:   "unavailable",
			Database: "unreachable",
		})
	}

	return web.JSON(w, http.StatusOK, readyResponse{
		Status:   "ok",
		Database: "reachable",
	})
}

// handleVersion reports what is actually deployed, which is the first question
// asked whenever behaviour does not match expectations.
func (a *API) handleVersion(w http.ResponseWriter, _ *http.Request) error {
	return web.JSON(w, http.StatusOK, buildVersion())
}

var (
	versionOnce  sync.Once
	versionCache versionResponse
)

// buildVersion reads the information the Go toolchain stamps into the binary,
// so nothing has to be injected with ldflags for this to be accurate.
func buildVersion() versionResponse {
	versionOnce.Do(func() {
		versionCache = versionResponse{
			Version:   "dev",
			Revision:  "unknown",
			BuildTime: "unknown",
			GoVersion: runtime.Version(),
		}

		info, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}

		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			versionCache.Version = info.Main.Version
		}

		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				versionCache.Revision = setting.Value
			case "vcs.time":
				versionCache.BuildTime = setting.Value
			case "vcs.modified":
				if setting.Value == "true" {
					versionCache.Revision += "-dirty"
				}
			}
		}
	})

	return versionCache
}
