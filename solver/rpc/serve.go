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
		AllowedOrigins:   []string{"*"},
		EnableReflection: false,
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

	// Add Prometheus metrics endpoint if enabled
	// The Prometheus exporter provides metrics through the promhttp handler
	if config.OTelConfig != nil && config.OTelConfig.UsePrometheus {
		mux.Handle("/metrics", promhttp.Handler())
	}

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
        AllowCredentials: true,
        MaxAge:          int(2 * time.Hour / time.Second),
        Debug:           *debug, // Set to true for debugging
    }).Handler(next)
}

//logging interceptor
func loggingInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Configure logging
			out := zerolog.ConsoleWriter{Out: os.Stderr}
			log := zerolog.New(out).With().Timestamp().Logger()

			start := time.Now()
            
            log.Info().
                Str("procedure", req.Spec().Procedure).
                Str("protocol", req.Peer().Protocol).
                Msg("request started")

            resp, err := next(ctx, req)
            
            duration := time.Since(start)
            
            if err != nil {
                log.Error().
                    Err(err).
                    Str("procedure", req.Spec().Procedure).
                    Dur("duration", duration).
                    Msg("request failed")
            } else {
                log.Info().
                    Str("procedure", req.Spec().Procedure).
                    Dur("duration", duration).
                    Msg("request completed")
            }

            return resp, err
        }
    }
}

// Start begins serving RPC requests
func (s *Server) Start() error {
	fmt.Printf("Starting RPC server on %s\n", s.config.Address)
	fmt.Printf("\tProtocols: gRPC, gRPC-Web, Connect\n")
	fmt.Printf("\tCORS origins: %v\n", s.config.AllowedOrigins)

	return s.httpServer.ListenAndServe()
}

// StartTLS begins serving RPC requests with TLS
func (s *Server) StartTLS(certFile, keyFile string) error {
	fmt.Printf("Starting RPC server on %s using TLS\n", s.config.Address)
	fmt.Printf("\tProtocols: gRPC, gRPC-Web, Connect\n")
	fmt.Printf("\tCORS origins: %v\n", s.config.AllowedOrigins)

	return s.httpServer.ListenAndServeTLS(certFile, keyFile)
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