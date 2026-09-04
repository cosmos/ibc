// SPDX-License-Identifier: Apache-2.0

package otel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/pkg/graceful"
)

// Provider owns the metrics pipeline and Prometheus HTTP server.
type Provider struct {
	logger    *slog.Logger
	meter     *metric.MeterProvider
	meterStop graceful.ShutdownFunc
}

const simpleMetricsPath = "/metrics"

// GlobalSetup sets up the OpenTelemetry pipeline globally (if enabled).
// Failure to setup is not fatal, but will be logged.
func GlobalSetup(ctx context.Context, cfg config.Observability, logger *slog.Logger) {
	logger = logger.With("module", "otel")

	if !cfg.Metrics {
		logger.Info("Observability is disabled")
		return
	}

	provider, err := New(ctx, cfg, logger)
	if err != nil {
		logger.Warn("Failed to setup observability. Skipping", "err", err)
		return
	}

	otel.SetMeterProvider(provider.meter)

	graceful.AddCallback(provider.Stop)
}

// New bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
// note: right now, only metrics are configured (logging and tracing are not)
// see: https://opentelemetry.io/docs/languages/go/getting-started/#add-opentelemetry-instrumentation
func New(_ context.Context, cfg config.Observability, logger *slog.Logger) (provider *Provider, err error) {
	meterProvider, meterStop, err := newMeterProvider(cfg, logger)
	if err != nil {
		// todo should we have some kind of NopProvider?
		return nil, err
	}

	p := &Provider{
		logger:    logger,
		meter:     meterProvider,
		meterStop: meterStop,
	}

	return p, nil
}

func (p *Provider) Stop() error {
	p.logger.Info("Shutting down observability pipeline")
	serverErr := p.meterStop()

	ctx, cancel := timeoutCtx()
	defer cancel()

	return errors.Join(serverErr, p.meter.Shutdown(ctx))
}

func newMeterProvider(
	cfg config.Observability,
	logger *slog.Logger,
) (*metric.MeterProvider, graceful.ShutdownFunc, error) {
	if cfg.Type == config.ObservabilitySimple {
		return newSimpleMeterProvider(cfg, logger.With("type", config.ObservabilitySimple))
	}

	// todo: implement OTEL
	return nil, nil, fmt.Errorf("unsupported observability type: %s", cfg.Type)
}

// in "simple" type, prometheus adapter endpoint is exposed instead of OTEL collector
func newSimpleMeterProvider(
	cfg config.Observability,
	logger *slog.Logger,
) (*metric.MeterProvider, graceful.ShutdownFunc, error) {
	registry := prometheus.NewRegistry()
	metricExporter, err := otelprometheus.New(otelprometheus.WithRegisterer(registry))
	if err != nil {
		return nil, nil, err
	}

	meterProvider := metric.NewMeterProvider(metric.WithReader(metricExporter))

	// create new mux + server
	mux := http.NewServeMux()
	mux.Handle(simpleMetricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	server := &http.Server{
		Handler:           mux,
		Addr:              cfg.ListenAddress,
		ReadHeaderTimeout: 3 * time.Second,
	}

	// check port availability
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		_ = meterProvider.Shutdown(context.Background())
		return nil, nil, fmt.Errorf("listenAddress: %w", err)
	}

	logger.Info("Starting metrics server", "address", server.Addr, "path", simpleMetricsPath)

	go func() {
		err := server.Serve(ln)
		switch err {
		case nil, http.ErrServerClosed:
			logger.Info("Metrics server stopped")
		default:
			logger.Error("Failed to serve metrics", "err", err)
		}
	}()

	stopFunc := func() error {
		ctx, cancel := timeoutCtx()
		defer cancel()
		return server.Shutdown(ctx)
	}

	return meterProvider, stopFunc, nil
}

func timeoutCtx() (context.Context, context.CancelFunc) {
	const timeout = 3 * time.Second
	return context.WithTimeout(context.Background(), timeout)
}
