package metrics

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ResponseSizeRange categorizes responses by size in bytes.
type ResponseSizeRange string

const (
	// ResponseSizeTiny represents responses with 0-10KB.
	ResponseSizeTiny ResponseSizeRange = "0-10KB"
	// ResponseSizeSmall represents responses with 10-100KB.
	ResponseSizeSmall ResponseSizeRange = "10-100KB"
	// ResponseSizeMedium represents responses with 100KB-1MB.
	ResponseSizeMedium ResponseSizeRange = "100KB-1MB"
	// ResponseSizeLarge represents responses with 1-10MB.
	ResponseSizeLarge ResponseSizeRange = "1-10MB"
	// ResponseSizeHuge represents responses with 10MB-1GB.
	ResponseSizeHuge ResponseSizeRange = "10MB-1GB"
	// ResponseSizeHugePlus represents responses with >1GB.
	ResponseSizeHugePlus ResponseSizeRange = ">1GB"
)

// GetResponseSizeRange returns a string representation of the response size range.
func GetResponseSizeRange(size int) string {
	switch {
	case size <= 10*1024: // 10KB
		return string(ResponseSizeTiny)
	case size <= 100*1024: // 100KB
		return string(ResponseSizeSmall)
	case size <= 1024*1024: // 1MB
		return string(ResponseSizeMedium)
	case size <= 10*1024*1024: // 10MB
		return string(ResponseSizeLarge)
	case size <= 1024*1024*1024: // 1GB
		return string(ResponseSizeHuge)
	default:
		return string(ResponseSizeHugePlus)
	}
}

// FieldCountRange categorizes requests by field count.
type FieldCountRange string

const (
	// FieldCountTiny represents requests with 0-5 fields.
	FieldCountTiny FieldCountRange = "0-5"
	// FieldCountSmall represents requests with 6-10 fields.
	FieldCountSmall FieldCountRange = "6-10"
	// FieldCountMedium represents requests with 11-20 fields.
	FieldCountMedium FieldCountRange = "11-20"
	// FieldCountLarge represents requests with 21-40 fields.
	FieldCountLarge FieldCountRange = "21-40"
	// FieldCountHuge represents requests with 41+ fields.
	FieldCountHuge FieldCountRange = "41+"
)

// GetFieldComplexityRange returns a string representation of the field count range.
func GetFieldComplexityRange(count int) string {
	switch {
	case count <= 5:
		return string(FieldCountTiny)
	case count <= 10:
		return string(FieldCountSmall)
	case count <= 20:
		return string(FieldCountMedium)
	case count <= 40:
		return string(FieldCountLarge)
	default:
		return string(FieldCountHuge)
	}
}

var (
	requestCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "graphql_request_total",
			Help: "Total number of requests on the graphql server, categorized by field count range and status.",
		},
		[]string{"response_size", "complexity", "status"},
	)
)

// Tracer provides a GraphQL middleware for collecting Prometheus metrics.
type Tracer struct{}

var _ interface {
	graphql.HandlerExtension
	graphql.ResponseInterceptor
} = Tracer{}

// ExtensionName returns the name of this extension.
func (a Tracer) ExtensionName() string {
	return "Prometheus"
}

// Validate validates the GraphQL schema.
func (a Tracer) Validate(schema graphql.ExecutableSchema) error {
	return nil
}

// InterceptResponse intercepts GraphQL responses to record metrics.
func (a Tracer) InterceptResponse(
	ctx context.Context,
	next graphql.ResponseHandler,
) *graphql.Response {
	response := next(ctx)
	sizeStat := "unknown"
	complexityStat := "unknown"
	statusStat := "success"

	// Calculate response size and increment appropriate counter
	if response != nil {
		sizeStat = GetResponseSizeRange(len(response.Data))

		if len(response.Errors) > 0 {
			statusStat = "with_errors"
		}
	}

	complexity := extension.GetComplexityStats(ctx)
	if complexity != nil {
		complexityStat = GetFieldComplexityRange(complexity.Complexity)
	}

	requestCounter.WithLabelValues(sizeStat, complexityStat, statusStat).Inc()

	return response
}
