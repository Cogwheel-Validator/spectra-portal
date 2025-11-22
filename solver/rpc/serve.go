package rpc

import (
	"context"
	"fmt"
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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

var logger zerolog.Logger

func init() {
	out := zerolog.ConsoleWriter{Out: os.Stderr}
	logger = zerolog.New(out).With().Timestamp().Logger()
}

// ServerConfig holds configuration for the RPC server
type ServerConfig struct {
	Address          string
	AllowedOrigins   []string
	EnableReflection bool
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
		AllowedOrigins:   []string{"http://localhost:3000"}, // Restrict to localhost:3000 for development
		EnableReflection: true,
		RatePerMinute:    &rateLimit,
		Burst:            &burst,
		OTelConfig:       DefaultOTelConfig(),
	}
}

// Server wraps the HTTP server and provides lifecycle management
type Server struct {
	config         *ServerConfig
	httpServer     *http.Server
	mux            *chi.Mux
	otelShutdown   func(context.Context) error
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
	if config.OTelConfig != nil {
		shutdown, err := NewOTelSDK(ctx, config.OTelConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OpenTelemetry: %w", err)
		}
		otelShutdown = shutdown
	}

	// use chi and append any middleware here
	mux := chi.NewMux()
	
	// Add OpenTelemetry HTTP instrumentation
	mux.Use(func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, "http-server")
	})
	
	mux.Use(middleware.Logger)
	mux.Use(middleware.Recoverer)
	mux.Use(middleware.RequestID)
	mux.Use(middleware.RealIP)
	mux.Use(middleware.Compress(5))
	mux.Use(middleware.Timeout(60 * time.Second))
	
	if config.RatePerMinute != nil {
		mux.Use(httprate.LimitByIP(*config.RatePerMinute, 1*time.Minute))
	}
	if config.Burst != nil {
		mux.Use(middleware.Throttle(*config.Burst))
	}
	
	// Prometheus metrics endpoint
	if config.OTelConfig != nil && config.OTelConfig.UsePrometheus {
		mux.Handle("/metrics", promhttp.Handler())
		logger.Info().Msg("Metrics endpoint: /metrics")
	}

	// Health check endpoint (for load balancer and other services)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","service":"spectra-ibc-hub"}`))
	})
	logger.Info().Msg("Health check: /health")

	// Readiness probe (Kubernetes readiness checks)
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Add actual readiness checks
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})
	logger.Info().Msg("Readiness probe: /ready")

	// Create the SolverServer implementation
	solverServer := NewSolverServer(solver, denomResolver)

	// Configure connect options for all handlers
	connectOpts := []connect.HandlerOption{
		// Add custom error handling
		connect.WithRecover(recoverHandler),
		// Add logging interceptor
		connect.WithInterceptors(loggingInterceptor()),
	}

	// Add OpenTelemetry tracing interceptor if enabled
	if config.OTelConfig != nil && config.OTelConfig.EnableTracing {
		otelInterceptor, err := otelconnect.NewInterceptor()
		if err != nil {
			return nil, fmt.Errorf("failed to create OTEL interceptor: %w", err)
		}
		connectOpts = append(connectOpts, connect.WithInterceptors(otelInterceptor))
	}

	// Register the SolverService handler
	// This automatically supports:
	// - gRPC protocol
	// - gRPC-Web protocol
	// - Connect protocol (JSON/Protobuf)
	path, handler := v1connect.NewSolverServiceHandler(solverServer, connectOpts...)
	mux.Handle(path, handler)

	// Add reflection endpoint
	if config.EnableReflection {
		reflector := grpcreflect.NewStaticReflector(
			v1connect.SolverServiceName,
		)
		reflectionPath, reflectionHandler := grpcreflect.NewHandlerV1(
			reflector, connectOpts...
		)
		mux.Handle(reflectionPath, reflectionHandler)
	}

	// Setup CORS for gRPC-Web support
	corsHandler := newCORSHandler(config.AllowedOrigins, mux, nil)

	// Create HTTP server with h2c support (HTTP/2 without TLS)
	// This is required for gRPC over plaintext
	httpServer := &http.Server{
		Addr:              config.Address,
		Handler:           h2c.NewHandler(corsHandler, &http2.Server{}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &Server{
		config:       config,
		httpServer:   httpServer,
		mux:          mux,
		otelShutdown: otelShutdown,
	}, nil
}

func newCORSHandler(
	allowedOrigins []string, 
	next http.Handler,
	debug *bool,
) http.Handler {
	if debug == nil {
		debug = new(bool)
		*debug = false
	}
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}
	
	// CORS spec forbids wildcard origins with credentials
	allowCredentials := true
	if len(allowedOrigins) == 1 && allowedOrigins[0] == "*" {
		allowCredentials = false
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
        MaxAge:          int(2 * time.Hour / time.Second),
        Debug:           *debug, // Set to true for debugging
    }).Handler(next)
}

//logging interceptor
func loggingInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {

			start := time.Now()
            
            logger.Info().
                Str("procedure", req.Spec().Procedure).
                Str("protocol", req.Peer().Protocol).
                Msg("request started")

            resp, err := next(ctx, req)
            
            duration := time.Since(start)
            
            if err != nil {
                logger.Error().
                    Err(err).
                    Str("procedure", req.Spec().Procedure).
                    Dur("duration", duration).
                    Msg("request failed")
            } else {
                logger.Info().
                    Str("procedure", req.Spec().Procedure).
                    Dur("duration", duration).
                    Msg("request completed")
            }

            return resp, err
        }
    }
}

// Start begins serving RPC requests without TLS
// Use this when:
// - Behind a reverse proxy (nginx, Caddy, Traefik) that handles TLS
// - Behind a service mesh (Istio, Linkerd, Consul) that handles mTLS
// - On internal networks where TLS is not required
func (s *Server) Start() error {
	s.logServerInfo("http")
	return s.httpServer.ListenAndServe()
}

// StartTLS begins serving RPC requests with TLS
// Use this for direct exposure to the internet without a reverse proxy
func (s *Server) StartTLS(certFile, keyFile string) error {
	s.logServerInfo("https")
	return s.httpServer.ListenAndServeTLS(certFile, keyFile)
}

// logServerInfo logs server startup information
func (s *Server) logServerInfo(protocol string) {
	logger.Info().Msgf("🚀 Spectra IBC Hub RPC Server starting")
	logger.Info().Msgf("   Address: %s://%s", protocol, s.config.Address)
	logger.Info().Msgf("   Protocols: gRPC, gRPC-Web, Connect (auto-detected)")
	
	// Log endpoints
	logger.Info().Msg("📍 Available endpoints:")
	logger.Info().Msg("   RPC: /rpc.v1.SolverService/* (public)")
	logger.Info().Msg("   Health: /health (public - for load balancers)")
	logger.Info().Msg("   Ready: /ready (public - for Kubernetes)")
	
	if s.config.OTelConfig != nil && s.config.OTelConfig.UsePrometheus {
		logger.Warn().Msg("   Metrics: /metrics (⚠️  RESTRICT TO INTERNAL)")
	}
	
	if s.config.EnableReflection {
		logger.Warn().Msg("   Reflection: enabled (⚠️  DISABLE IN PRODUCTION)")
	}
	
	// Log nginx reverse proxy example
	if protocol == "http" {
		logger.Info().Msg("")
		logger.Info().Msg("💡 Example nginx config to restrict /metrics:")
		logger.Info().Msg("   location /metrics {")
		logger.Info().Msg("     allow 10.0.0.0/8;  # Internal network")
		logger.Info().Msg("     deny all;")
		logger.Info().Msg("   }")
	}
	
	logger.Info().Msg("")
}

// Shutdown gracefully shuts down the server and OpenTelemetry
func (s *Server) Shutdown(ctx context.Context) error {
	fmt.Println("Shutting down RPC server...")
	
	// Shutdown HTTP server first
	if err := s.httpServer.Shutdown(ctx); err != nil {
		fmt.Printf("Error shutting down HTTP server: %v\n", err)
	}
	
	// Then shutdown OpenTelemetry to flush any pending telemetry
	if s.otelShutdown != nil {
		if err := s.otelShutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown OpenTelemetry: %w", err)
		}
	}
	
	return nil
}

// recoverHandler handles panics in RPC handlers
func recoverHandler(ctx context.Context, spec connect.Spec, header http.Header, p any) error {
	fmt.Printf("panic: %v\n", p)
	return connect.NewError(connect.CodeInternal, fmt.Errorf("internal server error"))
}