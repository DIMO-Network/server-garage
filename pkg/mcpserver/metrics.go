package mcpserver

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	toolCallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_tool_calls_total",
			Help: "Total number of MCP tool calls, categorized by tool and status.",
		},
		[]string{"tool", "status"},
	)

	toolDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "mcp_tool_duration_seconds",
			Help: "Duration of MCP tool calls in seconds.",
		},
		[]string{"tool"},
	)
)
