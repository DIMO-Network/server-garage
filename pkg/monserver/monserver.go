package monserver

import (
	"net/http"
	"net/http/pprof"
	runtimepprof "runtime/pprof"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

func NewMonitoringServer(logger *zerolog.Logger, enablePprof bool) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("healthy"))
	})

	mux.Handle("GET /metrics", promhttp.Handler())

	// Add pprof handlers if enabled
	if enablePprof {
		// Index page and base profiles
		mux.HandleFunc("GET /debug/pprof/", pprof.Index)
		mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)

		// add specialized profiles
		profiles := runtimepprof.Profiles()
		for _, profile := range profiles {
			mux.Handle("GET /debug/pprof/"+profile.Name(), pprof.Handler(profile.Name()))
		}
		if logger != nil {
			logger.Info().Str("endpoint", "GET /debug/pprof").Msg("pprof profiling enabled on monitoring server")
		}
	}

	return mux
}
