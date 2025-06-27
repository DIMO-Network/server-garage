package errorhandler

const (
	// CodeUnknown is the code for when an error occurred before your server could attempt to parse the given GraphQL operation.
	CodeUnknown = "UNKNOWN"
	// CodeGraphQLParseFailed is the code for when the GraphQL operation string contains a syntax error.
	CodeGraphQLParseFailed = "GRAPHQL_PARSE_FAILED"
	// CodeGraphQLValidationFailed is the code for when the GraphQL operation is not valid against the server's schema.
	CodeGraphQLValidationFailed = "GRAPHQL_VALIDATION_FAILED"
	// CodeBadUserInput is the code for when the GraphQL operation includes an invalid value for a field argument.
	CodeBadUserInput = "BAD_USER_INPUT"
	// CodeBadRequest is the code for when an error occurred before your server could attempt to parse the given GraphQL operation.
	CodeBadRequest = "BAD_REQUEST"
	// CodeInternalServerError is the code for when an error occurred before your server could attempt to parse the given GraphQL operation.
	CodeInternalServerError = "INTERNAL_SERVER_ERROR"
	// CodeNotFound is the code for when a resource was not found.
	CodeNotFound = "NOT_FOUND"
	// CodeUnauthorized is the code for when a authentication is required and has failed or has not been provided.
	CodeUnauthorized = "UNAUTHORIZED"
	// CodeForbidden is the code for when a user is not authorized to access a resource.
	CodeForbidden = "FORBIDDEN"
	// CodeTooManyRequests is the code for when a user has made too many requests.
	CodeTooManyRequests = "TOO_MANY_REQUESTS"
)
