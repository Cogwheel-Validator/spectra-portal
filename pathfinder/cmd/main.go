package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/config"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/brokers"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/router/brokers/osmosis"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/pathfinder/rpc"
	"github.com/rs/zerolog"
)

var log zerolog.Logger

func init() {
	// Initialize zerolog with console writer
	out := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	log = zerolog.New(out).With().Timestamp().Logger()

	// Share the logger with the RPC package
	rpc.SetLogger(log)
}

func main() {
	// Parse command line flags
	configRpc := flag.String("config-rpc", "./rpc-config.toml", "config file for the rpc server")
	configChains := flag.String("config-chains", "generated_configs/pathfinder_config.toml", "config file for the chains")
	flag.Parse()

	log.Info().
		Str("rpc_config", *configRpc).
		Str("chains_config", *configChains).
		Msg("Starting Spectra's Pathfinder")

	// Load RPC server configuration
	rpcConfig, err := config.NewDefaultRPCPathfinderConfigLoader().LoadRPCPathfinderConfig(*configRpc)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load RPC config")
	}

	// Load chain configurations
	chainLoader := config.NewChainConfigLoader()
	chains, err := chainLoader.LoadFromFile(*configChains)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load chain config")
	}

	log.Info().Int("count", len(chains)).Msg("Loaded chains")

	// Build the route index
	routeIndex := router.NewRouteIndex()
	if err := routeIndex.BuildIndex(chains); err != nil {
		log.Fatal().Err(err).Msg("Failed to build route index")
	}

	// Initialize broker clients
	brokerClients := make(map[string]brokers.BrokerClient)

	// Initialize Osmosis SQS broker if configured
	if rpcConfig.SqsMainUrl != "" {
		// Get Osmosis chain contract address
		var osmosisContractAddress string
		for _, chain := range chains {
			if chain.BrokerId == "osmosis-sqs" {
				osmosisContractAddress = chain.IBCHooksContract
				break
			}
		}

		var osmosisBroker *osmosis.SqsBroker
		if len(rpcConfig.BackupSqsUrls) > 0 {
			osmosisBroker = osmosis.NewSqsBrokerWithFailover(
				rpcConfig.SqsMainUrl,
				rpcConfig.BackupSqsUrls,
				osmosisContractAddress,
			)
			log.Info().
				Str("primary", rpcConfig.SqsMainUrl).
				Int("backups", len(rpcConfig.BackupSqsUrls)).
				Msg("Osmosis SQS broker initialized with failover")
		} else {
			osmosisBroker = osmosis.NewSqsBroker(rpcConfig.SqsMainUrl, osmosisContractAddress)
			log.Info().
				Str("url", rpcConfig.SqsMainUrl).
				Msg("Osmosis SQS broker initialized")
		}
		// Register with broker ID from config (osmosis-sqs) to match chain config broker_id
		brokerClients["osmosis-sqs"] = osmosisBroker
	}

	// Create the pathfinder
	pathfinder := router.NewPathfinder(chains, routeIndex, brokerClients)

	// Create denom resolver for the RPC server
	denomResolver := router.NewDenomResolver(routeIndex)
	denomResolver.SetChains(chains)

	// Create the RPC server configuration
	serverConfig := buildServerConfig(rpcConfig)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create the RPC server
	server, err := rpc.NewServer(ctx, serverConfig, pathfinder, denomResolver)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create RPC server")
	}

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		if err := server.Start(); err != nil {
			log.Error().Err(err).Msg("Server error")
			sigCh <- syscall.SIGTERM
		}
	}()

	// Wait for shutdown signal
	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Shutdown error")
	}

	// Close broker clients
	for name, client := range brokerClients {
		if closer, ok := client.(interface{ Close() }); ok {
			closer.Close()
			log.Info().Str("broker", name).Msg("Closed broker client")
		}
	}
}

// buildServerConfig converts the loaded RPCPathfinderConfig to rpc.ServerConfig
func buildServerConfig(cfg *config.RPCPathfinderConfig) *rpc.ServerConfig {
	serverConfig := &rpc.ServerConfig{
		Address:          cfg.Host + ":" + itoa(cfg.Port),
		AllowedOrigins:   cfg.AllowedOrigins,
		EnableReflection: cfg.EnableReflection,
		EnableMetrics:    cfg.UsePrometheus, // Enable metrics endpoint if prometheus is enabled
	}

	// Set rate limiting if configured
	if cfg.RatePerMinute > 0 {
		serverConfig.RatePerMinute = &cfg.RatePerMinute
	}
	if cfg.MaxConcurrentRequests > 0 {
		serverConfig.MaxConcurrentRequests = &cfg.MaxConcurrentRequests
	}

	// Set OpenTelemetry configuration if any telemetry is enabled
	if cfg.EnableTracing || cfg.EnableMetrics || cfg.EnableLogs || cfg.UsePrometheus {
		serverConfig.OTelConfig = &rpc.OTelConfig{
			ServiceName:    defaultString(cfg.ServiceName, "spectra-ibc-hub"),
			ServiceVersion: defaultString(cfg.ServiceVersion, "1.0.0"),
			Environment:    defaultString(cfg.Environment, "development"),
			EnableTracing:  cfg.EnableTracing,
			UseOTLPTraces:  cfg.UseOTLPTraces,
			OTLPTracesURL:  cfg.OTLPTracesURL,
			EnableMetrics:  cfg.EnableMetrics,
			UsePrometheus:  cfg.UsePrometheus,
			UseOTLPMetrics: cfg.UseOTLPMetrics,
			OTLPMetricsURL: cfg.OTLPMetricsURL,
			EnableLogs:     cfg.EnableLogs,
			UseOTLPLogs:    cfg.UseOTLPLogs,
			OTLPLogsURL:    cfg.OTLPLogsURL,
			InsecureOTLP:   cfg.InsecureOTLP,
		}
	}

	return serverConfig
}

// itoa converts int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	negative := i < 0
	if negative {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// defaultString returns the default value if s is empty
func defaultString(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
