package config

type RPCPathfinderConfig struct {
	// rpc configs
	Port int    `toml:"port"`
	Host string `toml:"host"`

	// CORS configs
	AllowedOrigins   []string `toml:"allowed_origins"`
	EnableReflection bool     `toml:"enable_reflection"`

	// rate limiting configs
	RatePerMinute         int `toml:"rate_per_minute"`
	MaxConcurrentRequests int `toml:"max_concurrent_requests"`

	// OpenTelemetry configs
	ServiceName    string `toml:"service_name"`
	ServiceVersion string `toml:"service_version"`
	Environment    string `toml:"environment"` // PROD, DEV, TEST, LOCAL
	EnableTracing  bool   `toml:"enable_tracing"`
	UseOTLPTraces  bool   `toml:"use_otlp_traces"`
	OTLPTracesURL  string `toml:"otlp_traces_url"`
	EnableMetrics  bool   `toml:"enable_metrics"`
	UsePrometheus  bool   `toml:"use_prometheus"`
	UseOTLPMetrics bool   `toml:"use_otlp_metrics"`
	OTLPMetricsURL string `toml:"otlp_metrics_url"`
	EnableLogs     bool   `toml:"enable_logs"`
	UseOTLPLogs    bool   `toml:"use_otlp_logs"`
	OTLPLogsURL    string `toml:"otlp_logs_url"`

	InsecureOTLP bool `toml:"insecure_otlp"`

	// Development mode uses stdout exporters
	DevelopmentMode bool `toml:"development_mode"`

	// Osmosis SQS config
	SqsURLs []string `toml:"sqs_urls"`
}
