package config

import (
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// LoadRPCPathfinderConfig loads the RPC pathfinder config from the given path
func LoadRPCPathfinderConfig(configPath *string) (*RPCPathfinderConfig, error) {
	v := viper.New()

	if configPath == nil {
		// if no file expect envs
		config, err := loadEnv(v)
		if err != nil {
			return nil, fmt.Errorf("failed to load env config: %w", err)
		}
		return config, nil
	} else {
		config, err := loadFile(v, *configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load file config: %w", err)
		}
		return config, nil
	}
}

func loadEnv(v *viper.Viper) (*RPCPathfinderConfig, error) {
	// godot might fail if .env file is missing but
	// env can be applied through docker, systmed or other means, so skip error
	_ = godotenv.Load()
	v.SetEnvPrefix("PATHFINDER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	bindEnvKeys(v)

	var config RPCPathfinderConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal env config: %w", err)
	}
	if err := verifyConfig(&config); err != nil {
		return nil, fmt.Errorf("failed to verify config: %w", err)
	}
	return &config, nil
}

// bindEnvKeys binds each config key to its env var so Unmarshal sees env values
// when no config file is loaded (env-only mode).
func bindEnvKeys(v *viper.Viper) {
	keys := []string{
		"port", "host", "allowed_origins", "enable_reflection",
		"rate_per_minute", "max_concurrent_requests",
		"service_name", "service_version", "environment",
		"enable_tracing", "use_otlp_traces", "otlp_traces_url",
		"enable_metrics", "use_prometheus", "use_otlp_metrics", "otlp_metrics_url",
		"enable_logs", "use_otlp_logs", "otlp_logs_url",
		"insecure_otlp", "development_mode", "sqs_urls",
	}
	for _, k := range keys {
		_ = v.BindEnv(k)
	}
}

func loadFile(v *viper.Viper, configPath string) (*RPCPathfinderConfig, error) {
	if !strings.HasSuffix(configPath, ".toml") {
		return nil, fmt.Errorf("config file must be a toml file")
	}

	v.SetConfigFile(configPath)
	v.SetConfigType("toml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config RPCPathfinderConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := verifyConfig(&config); err != nil {
		return nil, fmt.Errorf("failed to verify config: %w", err)
	}

	return &config, nil
}

func verifyConfig(config *RPCPathfinderConfig) error {
	if config.Port <= 0 || config.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	if config.Host == "" {
		return fmt.Errorf("host is required")
	}

	if len(config.AllowedOrigins) == 0 {
		return fmt.Errorf("allowed_origins is required")
	}

	if len(config.SqsURLs) == 0 {
		return fmt.Errorf("sqs_urls is required")
	}

	for _, url := range config.SqsURLs {
		if url == "" {
			return fmt.Errorf("sqs_urls must not be empty")
		}
	}

	return nil
}
