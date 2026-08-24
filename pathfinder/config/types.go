package config

// RPCPathfinderConfig holds the RPC server, CORS, and rate limiting configuration.
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

	// Ip Address Detection config serves to have some sort of control of who is accessing IpAddrDetect.
	//
	// For the pathfinder to know the location it needs to know how to handle this there are 4 options:
	// 1. off - it won't collect any IP and it won't use any sort of middleware for tracking.
	// 2. direct - it is exposed directly to internet with no proxies, gateways or CDNs in front.
	// 3. header - there is some program of CDN in front like Cloudflare, Nginx or something else.
	// 4. proxy - behind one or more proxies which IP is known to you
	IpAddrDetect string `toml:"ip_addr_detect" mapstructure:"ip_addr_detect"`
	// Depending on the IpAddrDetect option, you may need to provide additional information.
	// 1. If it is set to `off` or `direct` leave it empty.
	// 2. If it is set to header place a header that is assumed as a trusted option, example:
	// For Nginx: "X-Real-IP", for Cloudflare: "CF-Connecting-IP", for Apache: "X-Client-IP" etc...
	// 3. If set to proxy through AWS Cloudfront or similar. Fill it with the proper IP addresses, example:
	// "13.32.0.0/15", "52.46.0.0/18",
	IpOptions *[]string `toml:"ip_options" mapstructure:"ip_options"`

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
