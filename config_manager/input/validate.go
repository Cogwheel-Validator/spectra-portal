package input

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"
)

// ValidationError contains details about a validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult contains the results of validating a chain configuration.
type ValidationResult struct {
	ChainID  string
	IsValid  bool
	Errors   []error
	Warnings []string
}

// Validator validates human-readable chain configurations.
// Note: Network validation is skipped by default since the enriched.Builder
// performs thorough endpoint validation (version consensus, height sync, tx indexer).
// Use WithSkipNetworkCheck(false) to enable basic reachability checks here.
type Validator struct {
	httpClient       *http.Client
	skipNetCheck     bool
	allowedExplorers []AllowedExplorer
}

// ValidatorOption configures the validator.
type ValidatorOption func(*Validator)

// WithHTTPClient sets a custom HTTP client for network checks.
func WithHTTPClient(client *http.Client) ValidatorOption {
	return func(v *Validator) {
		v.httpClient = client
	}
}

// WithSkipNetworkCheck disables network reachability checks.
func WithSkipNetworkCheck(skip bool) ValidatorOption {
	return func(v *Validator) {
		v.skipNetCheck = skip
	}
}

// NewValidator creates a new configuration validator.
// Network checks are skipped by default - use WithSkipNetworkCheck(false) to enable.
func NewValidator(allowedExplorers []AllowedExplorer, opts ...ValidatorOption) *Validator {
	v := &Validator{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		skipNetCheck:     true, // Skip by default - builder does thorough validation
		allowedExplorers: allowedExplorers,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// SupportedChainTypes lists the chain types we currently support.
var SupportedChainTypes = []string{"cosmos"}

// Validate validates a single chain configuration.
func (v *Validator) Validate(config *ChainInput) *ValidationResult {
	result := &ValidationResult{
		ChainID: config.Chain.ID,
		IsValid: true,
	}

	// Required field validations
	v.validateRequired(config, result)

	// Type validations
	v.validateTypes(config, result)

	// Logical validations
	v.validateLogic(config, result)

	// Network validations (optional)
	if !v.skipNetCheck {
		v.validateNetwork(config, result)
	}

	result.IsValid = len(result.Errors) == 0
	return result
}

// ValidateAll validates all configurations and returns a map of results.
func (v *Validator) ValidateAll(configs map[string]*ChainInput) (map[string]*ValidationResult, error) {
	results := make(map[string]*ValidationResult)
	var hasErrors bool

	for chainID, config := range configs {
		result := v.Validate(config)
		results[chainID] = result
		if !result.IsValid {
			hasErrors = true
		}
	}

	if hasErrors {
		return results, errors.New("one or more configurations failed validation")
	}
	return results, nil
}

func (v *Validator) validateRequired(config *ChainInput, result *ValidationResult) {
	chain := config.Chain

	if chain.Name == "" {
		result.Errors = append(result.Errors, &ValidationError{"chain.name", "is required"})
	}
	if chain.ID == "" {
		result.Errors = append(result.Errors, &ValidationError{"chain.id", "is required"})
	}
	if chain.Type == "" {
		result.Errors = append(result.Errors, &ValidationError{"chain.type", "is required"})
	}
	if chain.Registry == "" {
		result.Errors = append(result.Errors, &ValidationError{"chain.registry", "is required"})
	}
	if chain.ExplorerURL == "" {
		result.Errors = append(result.Errors, &ValidationError{"chain.explorer_url", "is required"})
	}
	if chain.Bech32Prefix == "" {
		result.Errors = append(result.Errors, &ValidationError{"chain.bech32_prefix", "is required"})
	}
	if len(chain.RPCs) == 0 {
		result.Errors = append(result.Errors, &ValidationError{"chain.rpcs", "at least one RPC endpoint is required"})
	}
	if len(chain.Rest) == 0 {
		result.Errors = append(result.Errors, &ValidationError{"chain.rest", "at least one REST endpoint is required"})
	}

	// Validate tokens
	for i, token := range config.Tokens {
		prefix := fmt.Sprintf("token[%d]", i)
		if token.Denom == "" {
			result.Errors = append(result.Errors, &ValidationError{prefix + ".denom", "is required"})
		}
		if token.Name == "" {
			result.Errors = append(result.Errors, &ValidationError{prefix + ".name", "is required"})
		}
		if token.Symbol == "" {
			result.Errors = append(result.Errors, &ValidationError{prefix + ".symbol", "is required"})
		}
		if token.Icon == "" {
			result.Errors = append(result.Errors, &ValidationError{prefix + ".icon", "is required"})
		}
		if len(token.AllowedDestinations) > 1 && slices.Contains(token.AllowedDestinations, "none") {
			result.Errors = append(result.Errors, &ValidationError{prefix + ".allowed_destinations", "cannot contain none if more than one destination is specified"})
		}
	}
}

func (v *Validator) validateTypes(config *ChainInput, result *ValidationResult) {
	chain := config.Chain

	if chain.Type != "" && !slices.Contains(SupportedChainTypes, chain.Type) {
		result.Errors = append(result.Errors, &ValidationError{
			"chain.type",
			fmt.Sprintf("unsupported type '%s', must be one of: %v", chain.Type, SupportedChainTypes),
		})
	}

	if chain.Slip44 < 0 {
		result.Errors = append(result.Errors, &ValidationError{"chain.slip44", "must be non-negative"})
	}

	for i, token := range config.Tokens {
		if token.Exponent < 0 || token.Exponent > 18 {
			result.Errors = append(result.Errors, &ValidationError{
				fmt.Sprintf("token[%d].exponent", i),
				"must be between 0 and 18",
			})
		}
	}
}

func (v *Validator) validateLogic(config *ChainInput, result *ValidationResult) {
	chain := config.Chain

	// If it's a broker, broker_id is required
	if chain.IsBroker && chain.BrokerID == "" {
		result.Errors = append(result.Errors, &ValidationError{
			"chain.broker_id",
			"is required when chain.is_broker is true",
		})
	}

	// Check for duplicate token denoms
	seenDenoms := make(map[string]bool)
	for i, token := range config.Tokens {
		if seenDenoms[token.Denom] {
			result.Errors = append(result.Errors, &ValidationError{
				fmt.Sprintf("token[%d].denom", i),
				fmt.Sprintf("duplicate denom '%s'", token.Denom),
			})
		}
		seenDenoms[token.Denom] = true
	}

	// Warn if no tokens are defined
	if len(config.Tokens) == 0 {
		result.Warnings = append(result.Warnings, "no tokens defined - IBC routing may be limited")
	}

	// Validate endpoint URLs
	for i, rpc := range chain.RPCs {
		if rpc.URL == "" {
			result.Errors = append(result.Errors, &ValidationError{
				fmt.Sprintf("chain.rpcs[%d].url", i),
				"is required",
			})
		}
	}
	for i, rest := range chain.Rest {
		if rest.URL == "" {
			result.Errors = append(result.Errors, &ValidationError{
				fmt.Sprintf("chain.rest[%d].url", i),
				"is required",
			})
		}
	}

	// Validate keplr json file name
	if config.Chain.KeplrJSONFileName == nil || *config.Chain.KeplrJSONFileName == "" && config.Chain.KeplrChainConfig == nil {
		result.Errors = append(result.Errors, &ValidationError{
			"chain.keplr_chain_config",
			"is required when chain.keplr_json is empty",
		})
	}

	// Validate keplr chain config
	if config.Chain.KeplrChainConfig != nil {
		v.validateKeplrChainConfig(config, result)
	}

	// Validate explorer link
	if !verifyExplorerLink(config.Chain.ExplorerURL, v.allowedExplorers) {
		result.Errors = append(result.Errors, &ValidationError{
			"chain.explorer_url",
			"is not a valid allowed explorer link",
		})
	}
}

func (v *Validator) validateKeplrChainConfig(config *ChainInput, result *ValidationResult) {
	keplrConfig := config.Chain.KeplrChainConfig

	// check all values that are not nil and are empty
	validateStructs(reflect.ValueOf(keplrConfig), result)
}

func validateStructs(values reflect.Value, result *ValidationResult) {
	valuesIter := reflect.ValueOf(values).MapRange()
	for valuesIter.Next() {
		key := valuesIter.Key()
		value := valuesIter.Value()
		if key.Kind() == reflect.String && value.Kind() == reflect.String && value.String() == "" {
			result.Errors = append(result.Errors, &ValidationError{
				"chain.keplr_chain_config." + key.String(),
				"is required",
			})
		}
		if value.Kind() == reflect.Int && value.Int() < 0 {
			result.Errors = append(result.Errors, &ValidationError{
				"chain.keplr_chain_config." + key.String(),
				"must be non-negative",
			})
		}
		// recursively validate the struct
		if value.Kind() == reflect.Struct {
			validateStructs(value, result)
		}
	}
}

// validateNetwork validates the network reachability of the chain.
// this is not a deep network validation, it only checks if the endpoints are reachable.
//
// Parameters:
// - config: the chain configuration to validate
// - result: the validation result to store the warnings
//
// Returns:
// - nil if the network is reachable
// - result containing:
//   - Warnings: warnings about the network reachability
//   - Errors: errors about the network reachability if the network is not reachable
//   - IsValid: true if the network is reachable, false otherwise
//   - ChainID: the chain ID
func (v *Validator) validateNetwork(config *ChainInput, result *ValidationResult) {
	// Check explorer URL is reachable
	if config.Chain.ExplorerURL != "" {
		resp, err := v.httpClient.Head(config.Chain.ExplorerURL)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("explorer URL %s is not reachable: %v", config.Chain.ExplorerURL, err))
		} else {
			if err := resp.Body.Close(); err != nil {
				log.Printf("failed to close response body: %v", err)
			}
		}
	}

	// Check at least one RPC is reachable
	rpcReachable := false
	for _, rpc := range config.Chain.RPCs {
		resp, err := v.httpClient.Get(rpc.URL + "/status")
		if err == nil {
			if err := resp.Body.Close(); err != nil {
				log.Printf("failed to close response body: %v", err)
			}
			rpcReachable = true
			break
		}
	}
	if !rpcReachable && len(config.Chain.RPCs) > 0 {
		result.Warnings = append(result.Warnings, "no RPC endpoints are currently reachable")
	}

	// Check at least one REST is reachable
	restReachable := false
	for _, rest := range config.Chain.Rest {
		resp, err := v.httpClient.Get(rest.URL + "/cosmos/base/tendermint/v1beta1/node_info")
		if err == nil {
			if err := resp.Body.Close(); err != nil {
				log.Printf("failed to close response body: %v", err)
			}
			restReachable = true
			break
		}
	}
	if !restReachable && len(config.Chain.Rest) > 0 {
		result.Warnings = append(result.Warnings, "no REST endpoints are currently reachable")
	}
}

// verifyExplorerLink verifies if the explorer link is valid and allowed.
//
// Params:
// - url: the explorer link to verify
// - allowedExplorers: the list of allowed explorers
//
// Returns:
// - bool: true if the explorer link is valid and allowed, false otherwise
func verifyExplorerLink(url string, allowedExplorers []AllowedExplorer) bool {
	if !strings.HasPrefix(url, "https://") {
		return false
	}

	url = strings.TrimPrefix(url, "https://")
	urlParts := strings.Split(url, "/")
	domain := urlParts[0]

	for _, explorer := range allowedExplorers {
		// we also need to trim the baseURL to the domain
		explorerBaseURL := strings.TrimPrefix(explorer.BaseURL, "https://")
		explorerBaseURLParts := strings.Split(explorerBaseURL, "/")
		explorerDomain := explorerBaseURLParts[0]
		if explorerDomain == domain {
			return true
		}
	}
	return false
}
