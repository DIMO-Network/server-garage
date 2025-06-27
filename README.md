# Server Garage

![GitHub license](https://img.shields.io/badge/license-Apache%202.0-blue.svg)
[![GoDoc](https://godoc.org/github.com/DIMO-Network/server-garage?status.svg)](https://godoc.org/github.com/DIMO-Network/server-garage)
[![Go Report Card](https://goreportcard.com/badge/github.com/DIMO-Network/server-garage)](https://goreportcard.com/report/github.com/DIMO-Network/server-garage)

A Go library providing common server infrastructure components for DIMO.

## Overview

This repository contains reusable Go packages for building server applications with:

- **Monitoring Server**: HTTP server with health checks, metrics, and optional pprof profiling
- **Server Runner**: Utilities for gracefully starting and stopping HTTP, gRPC, and Fiber servers
- **GraphQL Error Handling**: Standardized error handling and presentation for GraphQL APIs
- **GraphQL Metrics**: Prometheus metrics collection for GraphQL requests
- **Rich Errors**: Enhanced error types with external messages and error codes

## Packages

- `pkg/monserver`: Monitoring server with health endpoints and metrics
- `pkg/runner`: Server lifecycle management utilities
- `pkg/gql/errorhandler`: GraphQL error handling and presentation
- `pkg/gql/metrics`: GraphQL request metrics collection
- `pkg/richerrors`: Enhanced error types with codes and external messages
