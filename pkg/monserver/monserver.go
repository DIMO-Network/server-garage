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
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("HEAD /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	})

	mux.Handle("/metrics", promhttp.Handler())

	// Add pprof handlers if enabled
	if enablePprof {
		pprofMux := http.NewServeMux()
		mux.Handle("/debug/pprof/", pprofMux)

		// Index page and base profiles
		pprofMux.HandleFunc("GET /", pprof.Index)
		pprofMux.HandleFunc("GET /cmdline", pprof.Cmdline)
		pprofMux.HandleFunc("GET /profile", pprof.Profile)
		pprofMux.HandleFunc("GET /symbol", pprof.Symbol)
		pprofMux.HandleFunc("GET /trace", pprof.Trace)

		// add specialized profiles
		profiles := runtimepprof.Profiles()
		for _, profile := range profiles {
			pprofMux.Handle("GET /"+profile.Name(), pprof.Handler(profile.Name()))
		}
		if logger != nil {
			logger.Info().Str("endpoint", "/debug/pprof").Msg("pprof profiling enabled on monitoring server")
		}
	}

	return mux
}
