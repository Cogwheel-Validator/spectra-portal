package config

type RPCPathfinderConfig struct {
	// rpc configs
	Port int    `toml:"port" mapstructure:"port"`
	Host string `toml:"host" mapstructure:"host"`

	// CORS configs
	AllowedOrigins   []string `toml:"allowed_origins" mapstructure:"allowed_origins"`
	EnableReflection bool     `toml:"enable_reflection" mapstructure:"enable_reflection"`

	// rate limiting configs
	RatePerMinute         int `toml:"rate_per_minute" mapstructure:"rate_per_minute"`
	MaxConcurrentRequests int `toml:"max_concurrent_requests" mapstructure:"max_concurrent_requests"`

	// OpenTelemetry configs
	ServiceName    string `toml:"service_name" mapstructure:"service_name"`
	ServiceVersion string `toml:"service_version" mapstructure:"service_version"`
	Environment    string `toml:"environment" mapstructure:"environment"` // PROD, DEV, TEST, LOCAL
	EnableTracing  bool   `toml:"enable_tracing" mapstructure:"enable_tracing"`
	UseOTLPTraces  bool   `toml:"use_otlp_traces" mapstructure:"use_otlp_traces"`
	OTLPTracesURL  string `toml:"otlp_traces_url" mapstructure:"otlp_traces_url"`
	EnableMetrics  bool   `toml:"enable_metrics" mapstructure:"enable_metrics"`
	UsePrometheus  bool   `toml:"use_prometheus" mapstructure:"use_prometheus"`
	UseOTLPMetrics bool   `toml:"use_otlp_metrics" mapstructure:"use_otlp_metrics"`
	OTLPMetricsURL string `toml:"otlp_metrics_url" mapstructure:"otlp_metrics_url"`
	EnableLogs     bool   `toml:"enable_logs" mapstructure:"enable_logs"`
	UseOTLPLogs    bool   `toml:"use_otlp_logs" mapstructure:"use_otlp_logs"`
	OTLPLogsURL    string `toml:"otlp_logs_url" mapstructure:"otlp_logs_url"`

	InsecureOTLP bool `toml:"insecure_otlp" mapstructure:"insecure_otlp"`

	// Development mode uses stdout exporters
	DevelopmentMode bool `toml:"development_mode" mapstructure:"development_mode"`

	// Osmosis SQS config
	SqsURLs []string `toml:"sqs_urls" mapstructure:"sqs_urls"`
}
