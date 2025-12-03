package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// OTelConfig configures OpenTelemetry exporters
type OTelConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string

	// Traces
	EnableTracing bool
	UseOTLPTraces bool   // Use OTLP for traces (Jaeger, Tempo, etc.)
	OTLPTracesURL string // Default: http://localhost:4318/v1/traces

	// Metrics
	EnableMetrics     bool
	UsePrometheus     bool                 // Expose /metrics endpoint
	UseOTLPMetrics    bool                 // Use OTLP for metrics
	OTLPMetricsURL    string               // Default: http://localhost:4318/v1/metrics
	PrometheusHandler *prometheus.Exporter // Will be set if Prometheus is enabled

	// Logs
	EnableLogs  bool
	UseOTLPLogs bool   // Use OTLP for logs (Loki, etc.)
	OTLPLogsURL string // Default: http://localhost:4318/v1/logs

	// Security
	// InsecureOTLP allows unencrypted connections to OTLP endpoints.
	// WARNING: Only set to true for local development or testing.
	// Production environments should use TLS-secured connections (leave false).
	InsecureOTLP bool

	// TLS Configuration for OTLP client connections (optional)
	// These are used when YOUR server connects TO the observability backend
	// This is SEPARATE from your RPC server's TLS certificate
	OTLPClientCertFile string // Path to client certificate (for mTLS)
	OTLPClientKeyFile  string // Path to client key (for mTLS)
	OTLPCACertFile     string // Path to CA certificate to verify server

	// Development mode uses stdout exporters
	DevelopmentMode bool
}

// DefaultOTelConfig returns a sensible default configuration
func DefaultOTelConfig() *OTelConfig {
	return &OTelConfig{
		ServiceName:     "spectra-ibc-hub",
		ServiceVersion:  "1.0.0",
		Environment:     "production",
		EnableTracing:   true,
		UseOTLPTraces:   true,
		OTLPTracesURL:   "http://localhost:4318/v1/traces",
		EnableMetrics:   true,
		UsePrometheus:   true,
		UseOTLPMetrics:  false,
		OTLPMetricsURL:  "http://localhost:4318/v1/metrics",
		EnableLogs:      false, // Keep false by default, zerolog handles app logs
		UseOTLPLogs:     false,
		OTLPLogsURL:     "http://localhost:4318/v1/logs",
		InsecureOTLP:    false, // Secure by default - must explicitly enable for local dev
		DevelopmentMode: false,
	}
}

// NewOTelSDK bootstraps the OpenTelemetry pipeline with the given configuration.
// If it does not return an error, make sure to call the shutdown function for proper cleanup.
func NewOTelSDK(ctx context.Context, config *OTelConfig) (func(context.Context) error, error) {
	if config == nil {
		config = DefaultOTelConfig()
	}

	var shutdownFuncs []func(context.Context) error
	var err error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Create resource with service information
	res, err := newResource(config)
	if err != nil {
		return shutdown, fmt.Errorf("failed to create resource: %w", err)
	}

	// Set up propagator for distributed tracing
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider if enabled
	if config.EnableTracing {
		tracerProvider, err := newTracerProvider(ctx, res, config)
		if err != nil {
			handleErr(err)
			return shutdown, err
		}
		shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
		otel.SetTracerProvider(tracerProvider)
	}

	// Set up meter provider if enabled
	if config.EnableMetrics {
		meterProvider, err := newMeterProvider(ctx, res, config)
		if err != nil {
			handleErr(err)
			return shutdown, err
		}
		shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
		otel.SetMeterProvider(meterProvider)
	}

	// Set up logger provider if enabled
	if config.EnableLogs {
		loggerProvider, err := newLoggerProvider(ctx, res, config)
		if err != nil {
			handleErr(err)
			return shutdown, err
		}
		shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
		global.SetLoggerProvider(loggerProvider)
	}

	return shutdown, nil
}

// newResource creates a resource with service information
func newResource(config *OTelConfig) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironmentName(config.Environment),
		),
	)
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// buildTLSConfig creates a TLS configuration for OTLP client connections
func buildTLSConfig(config *OTelConfig) (*tls.Config, error) {
	if config.InsecureOTLP {
		return nil, nil // No TLS config needed for insecure connections
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Load custom CA certificate if provided
	if config.OTLPCACertFile != "" {
		caCert, err := os.ReadFile(config.OTLPCACertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate for mTLS if provided
	if config.OTLPClientCertFile != "" && config.OTLPClientKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.OTLPClientCertFile, config.OTLPClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

func newTracerProvider(ctx context.Context, res *resource.Resource, config *OTelConfig) (*trace.TracerProvider, error) {
	var exporter trace.SpanExporter
	var err error

	if config.DevelopmentMode {
		// Use stdout exporter for development
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout trace exporter: %w", err)
		}
	} else if config.UseOTLPTraces {
		// Use OTLP exporter for production (works with Jaeger, Tempo, etc.)
		otlpOpts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(config.OTLPTracesURL),
		}

		// Configure TLS
		if config.InsecureOTLP {
			// Only use insecure connections if explicitly enabled (e.g., for local development)
			otlpOpts = append(otlpOpts, otlptracehttp.WithInsecure())
		} else {
			// Build TLS config (may include custom CA or client certs)
			tlsConfig, err := buildTLSConfig(config)
			if err != nil {
				return nil, fmt.Errorf("failed to build TLS config for traces: %w", err)
			}
			if tlsConfig != nil {
				otlpOpts = append(otlpOpts, otlptracehttp.WithTLSClientConfig(tlsConfig))
			}
		}

		exporter, err = otlptracehttp.New(ctx, otlpOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
		}
	} else {
		// No exporter configured
		return trace.NewTracerProvider(trace.WithResource(res)), nil
	}

	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter,
			trace.WithBatchTimeout(5*time.Second),
		),
		trace.WithResource(res),
	)
	return tracerProvider, nil
}

func newMeterProvider(ctx context.Context, res *resource.Resource, config *OTelConfig) (*metric.MeterProvider, error) {
	var readers []metric.Reader

	// Add Prometheus exporter if enabled (most common for pull-based metrics)
	if config.UsePrometheus {
		prometheusExporter, err := prometheus.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create Prometheus exporter: %w", err)
		}
		config.PrometheusHandler = prometheusExporter
		readers = append(readers, prometheusExporter)
	}

	// Add OTLP exporter if enabled (for push-based metrics)
	if config.UseOTLPMetrics {
		if config.DevelopmentMode {
			// Use stdout in development
			stdoutExporter, err := stdoutmetric.New()
			if err != nil {
				return nil, fmt.Errorf("failed to create stdout metric exporter: %w", err)
			}
			readers = append(readers, metric.NewPeriodicReader(stdoutExporter,
				metric.WithInterval(10*time.Second)))
		} else {
			// Use OTLP in production
			otlpOpts := []otlpmetrichttp.Option{
				otlpmetrichttp.WithEndpoint(config.OTLPMetricsURL),
			}

			// Configure TLS
			if config.InsecureOTLP {
				// Only use insecure connections if explicitly enabled (e.g., for local development)
				otlpOpts = append(otlpOpts, otlpmetrichttp.WithInsecure())
			} else {
				// Build TLS config (may include custom CA or client certs)
				tlsConfig, err := buildTLSConfig(config)
				if err != nil {
					return nil, fmt.Errorf("failed to build TLS config for metrics: %w", err)
				}
				if tlsConfig != nil {
					otlpOpts = append(otlpOpts, otlpmetrichttp.WithTLSClientConfig(tlsConfig))
				}
			}

			otlpExporter, err := otlpmetrichttp.New(ctx, otlpOpts...)
			if err != nil {
				return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
			}
			readers = append(readers, metric.NewPeriodicReader(otlpExporter,
				metric.WithInterval(60*time.Second)))
		}
	}

	if len(readers) == 0 {
		// No exporters configured, create a no-op provider
		return metric.NewMeterProvider(metric.WithResource(res)), nil
	}

	opts := []metric.Option{metric.WithResource(res)}
	for _, reader := range readers {
		opts = append(opts, metric.WithReader(reader))
	}

	meterProvider := metric.NewMeterProvider(opts...)
	return meterProvider, nil
}

func newLoggerProvider(ctx context.Context, res *resource.Resource, config *OTelConfig) (*log.LoggerProvider, error) {
	var exporter log.Exporter
	var err error

	if config.DevelopmentMode {
		// Use stdout exporter for development
		exporter, err = stdoutlog.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout log exporter: %w", err)
		}
	} else if config.UseOTLPLogs {
		// Use OTLP exporter for production (works with Loki, etc.)
		otlpOpts := []otlploghttp.Option{
			otlploghttp.WithEndpoint(config.OTLPLogsURL),
		}

		// Configure TLS
		if config.InsecureOTLP {
			// Only use insecure connections if explicitly enabled (e.g., for local development)
			otlpOpts = append(otlpOpts, otlploghttp.WithInsecure())
		} else {
			// Build TLS config (may include custom CA or client certs)
			tlsConfig, err := buildTLSConfig(config)
			if err != nil {
				return nil, fmt.Errorf("failed to build TLS config for logs: %w", err)
			}
			if tlsConfig != nil {
				otlpOpts = append(otlpOpts, otlploghttp.WithTLSClientConfig(tlsConfig))
			}
		}

		exporter, err = otlploghttp.New(ctx, otlpOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP log exporter: %w", err)
		}
	} else {
		// No exporter configured
		return log.NewLoggerProvider(log.WithResource(res)), nil
	}

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(exporter)),
		log.WithResource(res),
	)
	return loggerProvider, nil
}
