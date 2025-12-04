package rpc

import (
	"context"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/otelconnect"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/solver/router"
	v1connect "github.com/Cogwheel-Validator/spectra-ibc-hub/solver/rpc/v1/v1connect"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

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
	Address          string
	AllowedOrigins   []string
	EnableReflection bool
	EnableMetrics    bool
	RatePerMinute    *int
	Burst            *int
	OTelConfig       *OTelConfig // OpenTelemetry configuration
}

// DefaultServerConfig returns a default server configuration
func DefaultServerConfig() *ServerConfig {
	rateLimit := 100
	burst := 200
	return &ServerConfig{
		Address:          "localhost:8080",
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:8080"},
		EnableReflection: true,
		EnableMetrics:    true,
		RatePerMinute:    &rateLimit,
		Burst:            &burst,
		OTelConfig:       DefaultOTelConfig(),
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
	solver *router.Solver,
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

	// Add zerolog middleware (replaces chi's default logger)
	mux.Use(zerologMiddleware)

	// Add recovery middleware with zerolog
	mux.Use(zerologRecoverer)

	// Standard middleware
	mux.Use(middleware.RequestID)
	mux.Use(middleware.RealIP)
	mux.Use(middleware.Compress(5))
	mux.Use(middleware.Timeout(60 * time.Second))

	// Add OpenTelemetry HTTP instrumentation if tracing is enabled
	if config.OTelConfig != nil && config.OTelConfig.EnableTracing {
		mux.Use(otelHTTPMiddleware)
	}

	// Rate limiting
	if config.RatePerMinute != nil && *config.RatePerMinute > 0 {
		mux.Use(httprate.LimitByIP(*config.RatePerMinute, 1*time.Minute))
	}
	if config.Burst != nil && *config.Burst > 0 {
		mux.Use(middleware.Throttle(*config.Burst))
	}

	// Prometheus metrics endpoint - enabled by separate flag or OTel config
	metricsEnabled := config.EnableMetrics || (config.OTelConfig != nil && config.OTelConfig.UsePrometheus)
	if metricsEnabled {
		mux.Handle("/metrics", promhttp.Handler())
		Logger.Info().Msg("Metrics endpoint enabled: /metrics")
	}

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"spectra-ibc-hub"}`))
	})

	// Readiness probe
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	// Create the SolverServer implementation
	solverServer := NewSolverServer(solver, denomResolver)

	// Configure connect options
	connectOpts := []connect.HandlerOption{
		connect.WithRecover(recoverHandler),
		connect.WithInterceptors(loggingInterceptor()),
	}

	// Add OpenTelemetry tracing interceptor if enabled
	if config.OTelConfig != nil && config.OTelConfig.EnableTracing {
		otelInterceptor, err := otelconnect.NewInterceptor()
		if err != nil {
			Logger.Warn().Err(err).Msg("Failed to create OTEL interceptor, continuing without it")
		} else {
			connectOpts = append(connectOpts, connect.WithInterceptors(otelInterceptor))
		}
	}

	// Register the SolverService handler
	path, handler := v1connect.NewSolverServiceHandler(solverServer, connectOpts...)
	mux.Handle(path+"*", handler)

	// Add reflection endpoints (both v1 and v1alpha for compatibility)
	if config.EnableReflection {
		reflector := grpcreflect.NewStaticReflector(
			v1connect.SolverServiceName,
		)

		// Register v1 reflection (newer clients)
		v1Path, v1Handler := grpcreflect.NewHandlerV1(reflector, connectOpts...)
		mux.Handle(v1Path+"*", v1Handler)

		// Register v1alpha reflection (grpcurl and older clients)
		v1AlphaPath, v1AlphaHandler := grpcreflect.NewHandlerV1Alpha(reflector, connectOpts...)
		mux.Handle(v1AlphaPath+"*", v1AlphaHandler)

		Logger.Info().
			Str("v1_path", v1Path).
			Str("v1alpha_path", v1AlphaPath).
			Msg("gRPC reflection enabled")
	}

	// Setup CORS for gRPC-Web support
	corsHandler := newCORSHandler(config.AllowedOrigins, mux)

	// Create HTTP server with h2c support (HTTP/2 without TLS)
	httpServer := &http.Server{
		Addr:              config.Address,
		Handler:           h2c.NewHandler(corsHandler, &http2.Server{}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return &Server{
		config:       config,
		httpServer:   httpServer,
		mux:          mux,
		otelShutdown: otelShutdown,
	}, nil
}

// zerologMiddleware logs HTTP requests using zerolog
func zerologMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		// Log the request
		Logger.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", ww.Status()).
			Int("bytes", ww.BytesWritten()).
			Dur("duration", time.Since(start)).
			Str("remote", r.RemoteAddr).
			Msg("request")
	})
}

// zerologRecoverer recovers from panics and logs with zerolog
func zerologRecoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				Logger.Error().
					Interface("panic", rvr).
					Str("path", r.URL.Path).
					Msg("Recovered from panic")

				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// otelHTTPMiddleware wraps handlers with OpenTelemetry instrumentation
func otelHTTPMiddleware(next http.Handler) http.Handler {
	// Import at runtime to avoid dependency when not needed
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simple pass-through for now, actual OTel instrumentation
		// is handled by otelconnect interceptor for gRPC calls
		next.ServeHTTP(w, r)
	})
}

func newCORSHandler(allowedOrigins []string, next http.Handler) http.Handler {
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}

	// CORS spec forbids wildcard origins with credentials
	var allowCredentials bool
	if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
		allowCredentials = false
	} else {
		allowCredentials = true
	}

	return cors.New(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
		},
		AllowedHeaders: []string{
			"Accept",
			"Accept-Encoding",
			"Accept-Post",
			"Connect-Accept-Encoding",
			"Connect-Content-Encoding",
			"Connect-Protocol-Version",
			"Content-Encoding",
			"Content-Type",
			"Grpc-Accept-Encoding",
			"Grpc-Encoding",
			"Grpc-Message",
			"Grpc-Status",
			"Grpc-Status-Details-Bin",
			"Grpc-Timeout",
		},
		ExposedHeaders: []string{
			"Content-Encoding",
			"Connect-Content-Encoding",
			"Grpc-Encoding",
			"Grpc-Message",
			"Grpc-Status",
			"Grpc-Status-Details-Bin",
		},
		AllowCredentials: allowCredentials,
		MaxAge:           int(2 * time.Hour / time.Second),
	}).Handler(next)
}

// loggingInterceptor logs gRPC/Connect requests
func loggingInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()

			resp, err := next(ctx, req)

			duration := time.Since(start)

			event := Logger.Info()
			if err != nil {
				event = Logger.Error().Err(err)
			}

			event.
				Str("procedure", req.Spec().Procedure).
				Str("protocol", req.Peer().Protocol).
				Dur("duration", duration).
				Msg("rpc")

			return resp, err
		}
	}
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
		Msg("Spectra IBC Hub RPC Server starting")

	Logger.Info().Msg("Available endpoints:")
	Logger.Info().Msg("\tRPC: /rpc.v1.SolverService/*")
	Logger.Info().Msg("\tHealth: /health")
	Logger.Info().Msg("\tReady: /ready")

	if s.config.EnableMetrics || (s.config.OTelConfig != nil && s.config.OTelConfig.UsePrometheus) {
		Logger.Info().Msg("\tMetrics: /metrics")
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
	return connect.NewError(connect.CodeInternal, nil)
}
