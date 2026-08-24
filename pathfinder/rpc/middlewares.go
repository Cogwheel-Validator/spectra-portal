package rpc

import (
	"context"
	"net/http"
	"strings"
	"time"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/rs/cors"
	"google.golang.org/protobuf/proto"
)

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

// realIPMiddleware sets the remote address to the real IP address of the client
// This is useful for logging and rate limiting
//
// Deprecated: should be trackable by chi's new ClientIP middleware.
func realIPMiddleware(next http.Handler) http.Handler { //nolint:unused // ignore it it will be removed in future releases
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check CF-Connecting-IP first (most reliable with Cloudflare)
		if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
			r.RemoteAddr = ip
			next.ServeHTTP(w, r)
			return
		}

		// Fall back to X-Forwarded-For
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// X-Forwarded-For can contain multiple IPs, take the first one
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				ip := strings.TrimSpace(ips[0])
				r.RemoteAddr = ip
				next.ServeHTTP(w, r)
				return
			}
		}

		// No proxy headers found, use original RemoteAddr
		next.ServeHTTP(w, r)
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

func newCORSHandler(allowedOrigins []string) *cors.Cors {
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
			"Accept-Encoding",
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
	})
}

// loggingInterceptor logs gRPC/Connect requests with timing.
// It emits two log lines per call:
//   - "rpc received" at the moment the request arrives, with request details
//   - "rpc complete" (or "rpc error") once the handler returns, with elapsed duration
func loggingInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()

			Logger.Info().
				Str("procedure", req.Spec().Procedure).
				Str("protocol", req.Peer().Protocol).
				Str("peer", req.Peer().Addr).
				Msgf("rpc received: %+v", req.Any())

			resp, err := next(ctx, req)

			duration := time.Since(start)

			event := Logger.Info()
			msg := "rpc complete"
			if err != nil {
				event = Logger.Error().Err(err)
				msg = "rpc error"
			}

			event.
				Str("procedure", req.Spec().Procedure).
				Str("protocol", req.Peer().Protocol).
				Str("peer", req.Peer().Addr).
				Dur("duration", duration).
				Msg(msg)

			return resp, err
		}
	}
}

// noCacheInterceptor prevents caching of volatile responses like swap quotes.
// While NO_SIDE_EFFECTS is semantically correct (queries don't modify state),
// the results are time-sensitive and should not be cached by browsers/CDNs.
func noCacheInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err == nil && resp != nil {
				// Prevent caching of volatile data like swap quotes and routes
				resp.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
			}
			return resp, err
		}
	}
}

// validationInterceptor validates incoming requests using protovalidate.
// It checks all validation rules defined in the proto files (required fields,
// string length, numeric ranges, etc.) and returns InvalidArgument if validation fails.
func validationInterceptor(validator protovalidate.Validator) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Validate the request message if it implements the proto.Message interface
			if msgAny := req.Any(); msgAny != nil {
				if msg, ok := msgAny.(proto.Message); ok {
					if err := validator.Validate(msg); err != nil {
						Logger.Debug().
							Str("procedure", req.Spec().Procedure).
							Err(err).
							Msg("Request validation failed")
						return nil, connect.NewError(connect.CodeInvalidArgument, err)
					}
				}
			}
			return next(ctx, req)
		}
	}
}

func setupMiddleware(mux *chi.Mux, config *ServerConfig) {
	// CORS Middleware
	cors := newCORSHandler(config.AllowedOrigins)
	mux.Use(cors.Handler)

	// IP tracking and RequestID
	ipTracking(mux, config)
	mux.Use(middleware.RequestID)

	// Zerolog
	mux.Use(zerologMiddleware)
	mux.Use(zerologRecoverer)

	if config.OTelConfig != nil && config.OTelConfig.EnableTracing {
		mux.Use(otelHTTPMiddleware)
	}

	// TODO: this is localized and good enough for testing and local development
	// Add Valkey support in future releases
	if config.RatePerMinute != nil && *config.RatePerMinute > 0 {
		mux.Use(httprate.LimitBy(*config.RatePerMinute, 1*time.Minute, clientIpKey))
	}
}

// ipTracking sets up the IP tracking middleware based on the config
func ipTracking(mux *chi.Mux, config *ServerConfig) {
	switch config.IpAddrDetect {
	case "direct":
		mux.Use(middleware.ClientIPFromRemoteAddr)
	case "header":
		header := (*config.IpOptions)[0]
		mux.Use(middleware.ClientIPFromHeader(header))
	case "proxy":
		IPs := (*config.IpOptions)
		mux.Use(middleware.ClientIPFromXFF(IPs...))
	}
}

// clientIpKey returns the canonicalized IP address from the request
func clientIpKey(r *http.Request) (string, error) {
	return httprate.CanonicalizeIP(middleware.GetClientIP(r.Context())), nil
}
