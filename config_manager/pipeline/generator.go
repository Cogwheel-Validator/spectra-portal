// Package pipeline provides the main configuration generation pipeline that
// transforms human-readable configs into generated configs for backend and frontend.
package pipeline

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/enriched"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/output"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/registry"
	"github.com/pelletier/go-toml/v2"
)

// OutputFormat specifies the output format for generated configs.
type OutputFormat string

const (
	FormatTOML OutputFormat = "toml"
	FormatJSON OutputFormat = "json"
	FormatAuto OutputFormat = "auto" // Determine from file extension
)

// GeneratorConfig configures the pipeline generator.
type GeneratorConfig struct {
	// Path to the directory containing human-readable chain configs
	InputDir string

	// Path to output the generated solver config
	SolverOutputPath string

	// Path to output the generated client config
	ClientOutputPath string

	// Output format for solver config (default: auto from extension)
	SolverOutputFormat OutputFormat

	// Output format for client config (default: auto from extension)
	ClientOutputFormat OutputFormat

	// Path to cache the IBC registry data (optional)
	RegistryCachePath string

	// Base URL for chain logos in client config
	ChainLogoBaseURL string

	// Skip network validation of endpoints
	SkipNetworkValidation bool

	// Skip downloading fresh registry data (use cache)
	UseRegistryCache bool
}

// Generator is the main config generation pipeline.
type Generator struct {
	config         GeneratorConfig
	inputLoader    *input.Loader
	inputValidator *input.Validator
	enrichBuilder  *enriched.Builder
	solverConv     *output.SolverConverter
	clientConv     *output.ClientConverter
}

// NewGenerator creates a new pipeline generator with the given configuration.
func NewGenerator(config GeneratorConfig) *Generator {
	var clientConvOpts []output.ClientConverterOption
	if config.ChainLogoBaseURL != "" {
		clientConvOpts = append(clientConvOpts, output.WithChainLogoBaseURL(config.ChainLogoBaseURL))
	}

	// Builder handles all network validation (version consensus, height sync, tx indexer)
	var builderOpts []enriched.BuilderOption
	if config.SkipNetworkValidation {
		builderOpts = append(builderOpts, enriched.WithSkipNetworkCheck(true))
	}

	return &Generator{
		config:         config,
		inputLoader:    input.NewLoader(),
		inputValidator: input.NewValidator(),
		enrichBuilder:  enriched.NewBuilder(builderOpts...),
		solverConv:     output.NewSolverConverter(),
		clientConv:     output.NewClientConverter(clientConvOpts...),
	}
}

// GenerateResult contains the results of the generation process.
type GenerateResult struct {
	// Number of chains processed
	ChainsProcessed int

	// Validation results for each chain
	ValidationResults map[string]*input.ValidationResult

	// Path where solver config was written
	SolverConfigPath string

	// Path where client config was written
	ClientConfigPath string

	// Any warnings during generation
	Warnings []string
}

// Generate runs the complete configuration generation pipeline.
func (g *Generator) Generate() (*GenerateResult, error) {
	result := &GenerateResult{
		ValidationResults: make(map[string]*input.ValidationResult),
		Warnings:          make([]string, 0),
	}

	// Step 1: Load input configs
	log.Println("Loading chain configs...")
	inputConfigs, err := g.inputLoader.LoadAllConfigs(g.config.InputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load input configs: %w", err)
	}
	log.Printf("Loaded %d chain configs", len(inputConfigs))

	// Step 2: Validate input configs
	log.Println("Validating configs...")
	validationResults, _ := g.inputValidator.ValidateAll(inputConfigs)
	result.ValidationResults = validationResults

	validCount := 0
	for chainID, valResult := range validationResults {
		if valResult.IsValid {
			validCount++
		} else {
			log.Printf("%s: validation failed", chainID)
			for _, err := range valResult.Errors {
				log.Printf("\t- %v", err)
			}
		}
		for _, warning := range valResult.Warnings {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %s", chainID, warning))
		}
	}
	log.Printf("%d/%d chains passed validation", validCount, len(inputConfigs))

	// Step 3: Fetch IBC registry data
	log.Println("Fetching IBC registry data...")
	ibcData, err := g.fetchIBCRegistry(inputConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch IBC registry: %w", err)
	}
	log.Printf("Found %d IBC connections", len(ibcData))

	// Step 4: Build enriched configs (IBC denoms computed from config)
	log.Println("Building enriched configs...")
	enrichedReg, err := g.enrichBuilder.BuildRegistry(inputConfigs, ibcData)
	if err != nil {
		return nil, fmt.Errorf("failed to build enriched config: %w", err)
	}
	result.ChainsProcessed = len(enrichedReg.Chains)

	// Step 5: Generate solver config
	log.Println("Generating solver config...")
	solverConfig, err := g.solverConv.Convert(enrichedReg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to solver config: %w", err)
	}

	if g.config.SolverOutputPath != "" {
		if err := g.writeSolverConfig(solverConfig); err != nil {
			return nil, fmt.Errorf("failed to write solver config: %w", err)
		}
		result.SolverConfigPath = g.config.SolverOutputPath
		log.Printf("Written to %s", g.config.SolverOutputPath)
	}

	// Step 6: Generate client config
	log.Println("Generating client config...")
	clientConfig, err := g.clientConv.Convert(enrichedReg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to client config: %w", err)
	}

	if g.config.ClientOutputPath != "" {
		if err := g.writeClientConfig(clientConfig); err != nil {
			return nil, fmt.Errorf("failed to write client config: %w", err)
		}
		result.ClientConfigPath = g.config.ClientOutputPath
		log.Printf("Written to %s", g.config.ClientOutputPath)
	}

	log.Println("Config generation complete!")
	return result, nil
}

func (g *Generator) fetchIBCRegistry(inputConfigs map[string]*input.ChainInput) ([]registry.ChainIbcData, error) {
	keywords := g.inputLoader.GetRegistryKeywords(inputConfigs)
	log.Printf("Looking for IBC data matching: %v", keywords)

	// Determine cache path
	cachePath := g.config.RegistryCachePath
	if cachePath == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		cachePath = filepath.Join(currentDir, "ibc-registry")
	}

	// Download registry if not using cache or cache doesn't exist
	if !g.config.UseRegistryCache {
		if err := os.RemoveAll(cachePath); err != nil {
			return nil, fmt.Errorf("failed to clear registry cache: %w", err)
		}

		if err := registry.RegistryGitDownload(cachePath); err != nil {
			return nil, fmt.Errorf("failed to download IBC registry: %w", err)
		}
	}

	// Process registry data
	ibcData, err := registry.ProcessIbcRegistry(cachePath, keywords)
	if err != nil {
		return nil, fmt.Errorf("failed to process IBC registry: %w", err)
	}

	return ibcData, nil
}

func (g *Generator) writeSolverConfig(config *output.SolverConfig) error {
	dir := filepath.Dir(g.config.SolverOutputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	format := g.config.SolverOutputFormat
	if format == FormatAuto || format == "" {
		format = formatFromExtension(g.config.SolverOutputPath)
	}

	var data []byte
	var err error

	switch format {
	case FormatJSON:
		data, err = json.MarshalIndent(config, "", "  ")
	default:
		data, err = toml.Marshal(config)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal solver config: %w", err)
	}

	return os.WriteFile(g.config.SolverOutputPath, data, 0644)
}

func (g *Generator) writeClientConfig(config *output.ClientConfig) error {
	dir := filepath.Dir(g.config.ClientOutputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	format := g.config.ClientOutputFormat
	if format == FormatAuto || format == "" {
		format = formatFromExtension(g.config.ClientOutputPath)
	}

	var data []byte
	var err error

	switch format {
	case FormatTOML:
		data, err = toml.Marshal(config)
	default:
		// Default to JSON for client config (better for frontend)
		data, err = json.MarshalIndent(config, "", "  ")
	}

	if err != nil {
		return fmt.Errorf("failed to marshal client config: %w", err)
	}

	return os.WriteFile(g.config.ClientOutputPath, data, 0644)
}

// formatFromExtension determines output format from file extension.
func formatFromExtension(path string) OutputFormat {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		return FormatTOML
	case ".json":
		return FormatJSON
	default:
		return FormatTOML // Default to TOML
	}
}

// GenerateSolverOnly generates only the solver configuration.
func (g *Generator) GenerateSolverOnly() (*output.SolverConfig, error) {
	inputConfigs, err := g.inputLoader.LoadAllConfigs(g.config.InputDir)
	if err != nil {
		return nil, err
	}

	ibcData, err := g.fetchIBCRegistry(inputConfigs)
	if err != nil {
		return nil, err
	}

	enrichedReg, err := g.enrichBuilder.BuildRegistry(inputConfigs, ibcData)
	if err != nil {
		return nil, err
	}

	return g.solverConv.Convert(enrichedReg)
}

// GenerateClientOnly generates only the client configuration.
func (g *Generator) GenerateClientOnly() (*output.ClientConfig, error) {
	inputConfigs, err := g.inputLoader.LoadAllConfigs(g.config.InputDir)
	if err != nil {
		return nil, err
	}

	ibcData, err := g.fetchIBCRegistry(inputConfigs)
	if err != nil {
		return nil, err
	}

	enrichedReg, err := g.enrichBuilder.BuildRegistry(inputConfigs, ibcData)
	if err != nil {
		return nil, err
	}

	return g.clientConv.Convert(enrichedReg)
}
