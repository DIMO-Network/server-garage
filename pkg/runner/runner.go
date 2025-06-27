package runner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// NewSignalGroup creates a new context and error group that handles OS interrupt signals.
func NewSignalGroup(backgroundContext context.Context) (context.Context, *errgroup.Group) {
	ctx, cancel := signal.NotifyContext(backgroundContext, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		zerolog.Ctx(backgroundContext).Info().Msg("Received interrupt signal, shutting down...")
		cancel()
	}()
	group, gCtx := errgroup.WithContext(ctx)
	return gCtx, group
}

// FiberApp is an interface that represents a Fiber application.
type FiberApp interface {
	Listen(addr string) error
	Shutdown() error
}

// RunFiber starts a Fiber application in a new goroutine and shuts it down when the context is cancelled.
func RunFiber(ctx context.Context, fiberApp FiberApp, addr string, group *errgroup.Group) {
	group.Go(func() error {
		if err := fiberApp.Listen(addr); err != nil {
			return fmt.Errorf("failed to start server: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		<-ctx.Done()
		if err := fiberApp.Shutdown(); err != nil {
			return fmt.Errorf("failed to shutdown server: %w", err)
		}
		return nil
	})
}

// GRPCServer is an interface that represents a gRPC server.
type GRPCServer interface {
	Serve(lis net.Listener) error
	GracefulStop()
}

// RunGRPC starts a gRPC server in a new goroutine and shuts it down when the context is cancelled.
func RunGRPC(ctx context.Context, grpcServer GRPCServer, addr string, group *errgroup.Group) {
	group.Go(func() error {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to listen on gRPC port %s: %w", addr, err)
		}
		if err := grpcServer.Serve(lis); err != nil {
			return fmt.Errorf("gRPC server failed to serve: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		<-ctx.Done()
		grpcServer.GracefulStop()
		return nil
	})
}

// RunHandler starts a HTTP server in a new goroutine and shuts it down when the context is cancelled.
func RunHandler(ctx context.Context, handler http.Handler, addr string, group *errgroup.Group) {
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	group.Go(func() error {
		if err := srv.ListenAndServe(); err != nil {
			return fmt.Errorf("failed to run server: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		<-ctx.Done()
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown server: %w", err)
		}
		return nil
	})
}
