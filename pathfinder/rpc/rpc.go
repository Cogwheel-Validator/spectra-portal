package rpc

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/otelconnect"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/common"
	v1handlers "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/v1"
	v2betahandlers "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/handlers/v2beta"
	v1connect "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v1/v1connect"
	v2betaconnect "github.com/Cogwheel-Validator/spectra-portal/pathfinder/rpc/services/pathfinder/v2beta/v2betaconnect"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// Logger is the package-level logger used by the RPC server. Use SetLogger to replace it.
var Logger zerolog.Logger

func init() {
	out := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	Logger = zerolog.New(out).With().Timestamp().Logger()
}

// SetLogger allows setting a custom logger
func SetLogger(l zerolog.Logger) {
	Logger = l
}

// ServerConfig holds configuration for the RPC server
type ServerConfig struct {
	Address               string
	AllowedOrigins        []string
	EnableReflection      bool
	EnableMetrics         bool
	RatePerMinute         *int
	MaxConcurrentRequests *int
	OTelConfig            *OTelConfig // OpenTelemetry configuration
}

// DefaultServerConfig returns a default server configuration
func DefaultServerConfig() *ServerConfig {
	rateLimit := 0
	maxConcurrentRequests := 200
	return &ServerConfig{
		Address:               "localhost:8080",
		AllowedOrigins:        []string{"http://localhost:3000", "http://localhost:8080"},
		EnableReflection:      true,
		EnableMetrics:         true,
		RatePerMinute:         &rateLimit,
		MaxConcurrentRequests: &maxConcurrentRequests,
		OTelConfig:            DefaultOTelConfig(),
	}
}

// Server wraps the HTTP server and provides lifecycle management
type Server struct {
	config       *ServerConfig
	httpServer   *http.Server
	mux          *chi.Mux
	otelShutdown func(context.Context) error
}

// NewServer creates a new RPC server with the given configuration
func NewServer(
	ctx context.Context,
	config *ServerConfig,
	pathfinder *router.Pathfinder,
	denomResolver *router.DenomResolver,
) (*Server, error) {
	if config == nil {
		config = DefaultServerConfig()
	}

	// Initialize OpenTelemetry if configured
	var otelShutdown func(context.Context) error
	if config.OTelConfig != nil && (config.OTelConfig.EnableTracing || config.OTelConfig.EnableMetrics || config.OTelConfig.EnableLogs) {
		shutdown, err := NewOTelSDK(ctx, config.OTelConfig)
		if err != nil {
			Logger.Error().Err(err).Msg("Failed to initialize OpenTelemetry")
			// Don't fail the server, just continue without OTel
		} else {
			otelShutdown = shutdown
		}
	}

	// Create chi router
	mux := chi.NewMux()

	// Global middleware — applies to every route including streaming
	mux.Use(zerologMiddleware)
	mux.Use(zerologRecoverer)
	mux.Use(middleware.RequestID)
	mux.Use(middleware.RealIP)
	mux.Use(realIPMiddleware)

	if config.OTelConfig != nil && config.OTelConfig.EnableTracing {
		mux.Use(otelHTTPMiddleware)
	}

	if config.RatePerMinute != nil && *config.RatePerMinute > 0 {
		mux.Use(httprate.LimitByIP(*config.RatePerMinute, 1*time.Minute))
	}

	// Health/ready/metrics respond instantly — no timeout group needed
	metricsEnabled := config.EnableMetrics || (config.OTelConfig != nil && config.OTelConfig.UsePrometheus)
	if metricsEnabled {
		mux.Handle("/server/metrics", promhttp.Handler())
		Logger.Info().Msg("Metrics endpoint enabled: /server/metrics")
	}
	mux.HandleFunc("/server/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"pathfinder-rpc"}`))
	})
	mux.HandleFunc("/server/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	// Bundle the dependencies shared by all versioned handlers
	deps := common.Deps{
		Pathfinder:    pathfinder,
		DenomResolver: denomResolver,
		Logger:        Logger,
	}

	// Initialize protovalidate validator
	validator, err := protovalidate.New()
	if err != nil {
		Logger.Warn().Err(err).Msg("Failed to initialize protovalidate, continuing without validation")
		validator = nil
	} else {
		Logger.Info().Msg("Protovalidate initialized successfully")
	}

	// Configure connect options
	connectOpts := []connect.HandlerOption{
		connect.WithRecover(recoverHandler),
		connect.WithInterceptors(
			loggingInterceptor(),
			noCacheInterceptor(),
		),
	}
	if validator != nil {
		connectOpts = append(connectOpts, connect.WithInterceptors(validationInterceptor(validator)))
	}
	if config.OTelConfig != nil && config.OTelConfig.EnableTracing {
		otelInterceptor, err := otelconnect.NewInterceptor()
		if err != nil {
			Logger.Warn().Err(err).Msg("Failed to create OTEL interceptor, continuing without it")
		} else {
			connectOpts = append(connectOpts, connect.WithInterceptors(otelInterceptor))
		}
	}

	v1Path, v1Handler := v1connect.NewPathfinderServiceHandler(v1handlers.NewServer(deps), connectOpts...)
	_, findPathV2Handler := v2betaconnect.NewFindPathServiceHandler(v2betahandlers.NewFindPathServer(deps), connectOpts...)
	queryV2Path, queryV2Handler := v2betaconnect.NewPathfinderQueryServiceHandler(v2betahandlers.NewQueryServer(deps), connectOpts...)

	mux.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(60 * time.Second))
		if config.MaxConcurrentRequests != nil && *config.MaxConcurrentRequests > 0 {
			r.Use(middleware.Throttle(*config.MaxConcurrentRequests))
		}
		r.Handle(v1Path+"*", v1Handler)
		// v2beta FindPath is mounted per procedure so that the streaming
		// procedure can live outside the timeout/throttle group.
		r.Handle(v2betaconnect.FindPathServiceFindPathProcedure, findPathV2Handler)
		r.Handle(queryV2Path+"*", queryV2Handler)

		if config.EnableReflection {
			reflector := grpcreflect.NewStaticReflector(
				v1connect.PathfinderServiceName,
				v2betaconnect.FindPathServiceName,
				v2betaconnect.PathfinderQueryServiceName,
			)
			reflectPath, reflectHandler := grpcreflect.NewHandlerV1(reflector, connectOpts...)
			r.Handle(reflectPath+"*", reflectHandler)
			reflectAlphaPath, reflectAlphaHandler := grpcreflect.NewHandlerV1Alpha(reflector, connectOpts...)
			r.Handle(reflectAlphaPath+"*", reflectAlphaHandler)
			Logger.Info().Msg("gRPC reflection enabled")
		}
	})

	// Streaming handler — no timeout, no throttle
	mux.Handle(v2betaconnect.FindPathServiceFindPathStreamingProcedure, findPathV2Handler)

	// Setup CORS for gRPC-Web support
	corsHandler := newCORSHandler(config.AllowedOrigins, mux)

	httpServer := &http.Server{
		Addr:              config.Address,
		Handler:           h2c.NewHandler(corsHandler, &http2.Server{}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       0,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
	}

	return &Server{
		config:       config,
		httpServer:   httpServer,
		mux:          mux,
		otelShutdown: otelShutdown,
	}, nil
}

// Start begins serving RPC requests without TLS
func (s *Server) Start() error {
	s.logServerInfo("http")
	return s.httpServer.ListenAndServe()
}

// StartTLS begins serving RPC requests with TLS
func (s *Server) StartTLS(certFile, keyFile string) error {
	s.logServerInfo("https")
	return s.httpServer.ListenAndServeTLS(certFile, keyFile)
}

// logServerInfo logs server startup information
func (s *Server) logServerInfo(protocol string) {
	Logger.Info().
		Str("address", s.config.Address).
		Str("protocol", protocol).
		Msg("Spectra's Pathfinder RPC Server starting")

	Logger.Info().Msg("Available endpoints:")
	Logger.Info().Msg("\tRPC v1: /pathfinder.v1.PathfinderService/*")
	Logger.Info().Msg("\tRPC v2beta: /pathfinder.v2beta.FindPathService/* (unary + streaming)")
	Logger.Info().Msg("\tRPC v2beta: /pathfinder.v2beta.PathfinderQueryService/*")
	Logger.Info().Msg("\tHealth: /server/health")
	Logger.Info().Msg("\tReady: /server/ready")

	if s.config.EnableMetrics || (s.config.OTelConfig != nil && s.config.OTelConfig.UsePrometheus) {
		Logger.Info().Msg("\tMetrics: /server/metrics")
	}

	if s.config.EnableReflection {
		Logger.Warn().Msg("\tReflection: enabled (consider disabling in production)")
	}
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	Logger.Info().Msg("Shutting down RPC server...")

	// Shutdown HTTP server first
	if err := s.httpServer.Shutdown(ctx); err != nil {
		Logger.Error().Err(err).Msg("Error shutting down HTTP server")
	}

	// Then shutdown OpenTelemetry to flush any pending telemetry
	if s.otelShutdown != nil {
		if err := s.otelShutdown(ctx); err != nil {
			Logger.Error().Err(err).Msg("Error shutting down OpenTelemetry")
			return err
		}
	}

	Logger.Info().Msg("Server shutdown complete")
	return nil
}

// recoverHandler handles panics in RPC handlers
func recoverHandler(ctx context.Context, spec connect.Spec, header http.Header, p any) error {
	Logger.Error().
		Interface("panic", p).
		Str("procedure", spec.Procedure).
		Msg("Panic in RPC handler")
	// Return a generic error message to clients (don't expose internal panic details)
	return connect.NewError(connect.CodeInternal, fmt.Errorf("internal server error"))
}
