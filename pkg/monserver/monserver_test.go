package monserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewMonitoringServer(t *testing.T) {
	tests := []struct {
		name        string
		enablePprof bool
		endpoints   []struct {
			path   string
			method string
			want   int
			body   string
		}
	}{
		{
			name:        "basic endpoints without pprof",
			enablePprof: false,
			endpoints: []struct {
				path   string
				method string
				want   int
				body   string
			}{
				{path: "/", method: "GET", want: http.StatusOK, body: "ok"},
				{path: "/health", method: "GET", want: http.StatusOK, body: "healthy"},
				{path: "/metrics", method: "GET", want: http.StatusOK, body: ""}, // Prometheus metrics
				{path: "/nonexistent", method: "GET", want: http.StatusNotFound, body: ""},
			},
		},
		{
			name:        "basic endpoints with pprof",
			enablePprof: true,
			endpoints: []struct {
				path   string
				method string
				want   int
				body   string
			}{
				{path: "/", method: "GET", want: http.StatusOK, body: "ok"},
				{path: "/health", method: "GET", want: http.StatusOK, body: "healthy"},
				{path: "/metrics", method: "GET", want: http.StatusOK, body: ""},      // Prometheus metrics
				{path: "/debug/pprof/", method: "GET", want: http.StatusOK, body: ""}, // pprof index
				{path: "/debug/pprof/cmdline", method: "GET", want: http.StatusOK, body: ""},
				{path: "/debug/pprof/profile?seconds=1", method: "GET", want: http.StatusOK, body: ""}, // 1 second profile
				{path: "/debug/pprof/symbol", method: "GET", want: http.StatusOK, body: ""},
				{path: "/debug/pprof/trace", method: "GET", want: http.StatusOK, body: ""},
				{path: "/debug/pprof/goroutine", method: "GET", want: http.StatusOK, body: ""},
				{path: "/debug/pprof/heap", method: "GET", want: http.StatusOK, body: ""},
				{path: "/debug/pprof/threadcreate", method: "GET", want: http.StatusOK, body: ""},
				{path: "/debug/pprof/block", method: "GET", want: http.StatusOK, body: ""},
				{path: "/debug/pprof/mutex", method: "GET", want: http.StatusOK, body: ""},
				{path: "/nonexistent", method: "GET", want: http.StatusNotFound, body: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger := zerolog.New(zerolog.NewTestWriter(t))
			mux := NewMonitoringServer(&logger, tt.enablePprof)

			for _, endpoint := range tt.endpoints {
				t.Run(endpoint.path, func(t *testing.T) {
					t.Parallel()
					req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
					w := httptest.NewRecorder()

					mux.ServeHTTP(w, req)

					if w.Code != endpoint.want {
						t.Errorf("expected status %d, got %d", endpoint.want, w.Code)
					}

					if endpoint.body != "" {
						body := strings.TrimSpace(w.Body.String())
						if body != endpoint.body {
							t.Errorf("expected body %q, got %q", endpoint.body, body)
						}
					}

					// Check content type for specific endpoints
					switch endpoint.path {
					case "/", "/health":
						contentType := w.Header().Get("Content-Type")
						if contentType != "text/plain" {
							t.Errorf("expected Content-Type text/plain, got %s", contentType)
						}
					case "/metrics":
						contentType := w.Header().Get("Content-Type")
						if !strings.Contains(contentType, "text/plain") {
							t.Errorf("expected Content-Type to contain text/plain, got %s", contentType)
						}
					}
				})
			}
		})
	}
}

func TestMonitoringServerWithNilLogger(t *testing.T) {
	// Test that the server works correctly with a nil logger
	mux := NewMonitoringServer(nil, true)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "healthy" {
		t.Errorf("expected body 'healthy', got %q", w.Body.String())
	}
}

func TestMonitoringServerPprofDisabled(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	mux := NewMonitoringServer(&logger, false)

	// Test that pprof endpoints return 404 when disabled
	pprofEndpoints := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile?seconds=1", // 1 second profile
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
		"/debug/pprof/goroutine",
		"/debug/pprof/heap",
	}

	for _, endpoint := range pprofEndpoints {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest("GET", endpoint, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Errorf("expected status %d for disabled pprof endpoint %s, got %d",
					http.StatusNotFound, endpoint, w.Code)
			}
		})
	}
}
