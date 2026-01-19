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

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/cp"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/enriched"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/input"
	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/keplr"
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

	// Path to output the generated pathfinder config
	PathfinderOutputPath string

	// Path to output the generated client config
	ClientOutputPath string

	// Output format for pathfinder config (default: auto from extension)
	PathfinderOutputFormat OutputFormat

	// Output format for client config (default: auto from extension)
	ClientOutputFormat OutputFormat

	// Path to stored the IBC registry data (optional)
	LocalIbcRegistryPath string

	// Path to stored the Keplr registry data (optional)
	LocalKeplrRegistryPath string

	// Skip network validation of endpoints
	SkipNetworkValidation bool

	// Skip downloading fresh registry data (use stored data)
	UseLocalIbcReg bool

	// Skip downloading fresh keplr registry data (use stored data)
	UseLocalKeplrReg bool

	// If the path is set for this option the program will assume this is enabled and will try to copy the icons.
	CopyIconsPath string

	// Path to the allowed explorers file
	AllowedExplorersPath string
}

// Generator is the main config generation pipeline.
type Generator struct {
	config         GeneratorConfig
	inputLoader    *input.Loader
	inputValidator *input.Validator
	enrichBuilder  *enriched.Builder
	pathfinderConv *output.PathfinderConverter
	clientConv     *output.ClientConverter
}

// NewGenerator creates a new pipeline generator with the given configuration.
func NewGenerator(config GeneratorConfig) *Generator {
	var clientConvOpts []output.ClientConverterOption
	// Leave empty for now, it might be needed later on...

	// Builder handles all network validation (version consensus, height sync, tx indexer)
	var builderOpts []enriched.BuilderOption
	if config.SkipNetworkValidation {
		builderOpts = append(builderOpts, enriched.WithSkipNetworkCheck(true))
	}

	if config.CopyIconsPath != "" {
		clientConvOpts = append(clientConvOpts, output.WithIconCopy(true))
	}

	// init the input loader
	inputLoader := input.NewLoader()

	// init the input validator by calling upon the input loader to load the allowed explorers
	allowedExplorers, err := inputLoader.LoadListOfAllowedExplorers(config.AllowedExplorersPath)
	if err != nil {
		log.Fatalf("failed to load allowed explorers: %v", err)
	}
	inputValidator := input.NewValidator(allowedExplorers)

	return &Generator{
		config:         config,
		inputLoader:    inputLoader,
		inputValidator: inputValidator,
		enrichBuilder:  enriched.NewBuilder(allowedExplorers, builderOpts...),
		pathfinderConv: output.NewPathfinderConverter(),
		clientConv:     output.NewClientConverter(clientConvOpts...),
	}
}

// GenerateResult contains the results of the generation process.
type GenerateResult struct {
	// Number of chains processed
	ChainsProcessed int

	// Validation results for each chain
	ValidationResults map[string]*input.ValidationResult

	// Path where pathfinder config was written
	PathfinderConfigPath string

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

	// Step 3: Fetch IBC registry data and Keplr registry data
	log.Println("Fetching IBC and Keplr registry data...")
	ibcData, err := g.fetchIBCRegistry(inputConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch IBC registry: %w", err)
	}
	log.Printf("Found %d IBC connections", len(ibcData))
	keplrConfigs, err := g.fetchKeplrRegistry(inputConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Keplr registry: %w", err)
	}
	log.Printf("Found %d Keplr chains", len(keplrConfigs))

	// Step 4: Build enriched configs (IBC denoms computed from config)
	log.Println("Building enriched configs...")
	enrichedReg, err := g.enrichBuilder.BuildRegistry(inputConfigs, ibcData, keplrConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to build enriched config: %w", err)
	}
	result.ChainsProcessed = len(enrichedReg.Chains)

	// Step 5: Generate pathfinder config
	log.Println("Generating pathfinder config...")
	pathfinderConfig, err := g.pathfinderConv.Convert(enrichedReg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to pathfinder config: %w", err)
	}

	if g.config.PathfinderOutputPath != "" {
		if err := g.writePathfinderConfig(pathfinderConfig); err != nil {
			return nil, fmt.Errorf("failed to write pathfinder config: %w", err)
		}
		result.PathfinderConfigPath = g.config.PathfinderOutputPath
		log.Printf("Written to %s", g.config.PathfinderOutputPath)
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

	// Step 7 (Optional): Copy icons to the public/icons directory
	if g.config.CopyIconsPath != "" {
		if err := g.copyChainImages(inputConfigs, enrichedReg); err != nil {
			return nil, fmt.Errorf("failed to copy icons: %w", err)
		}
		log.Printf("Copied icons to %s/icons", g.config.CopyIconsPath)
	}

	log.Println("Config generation complete!")
	return result, nil
}

func (g *Generator) fetchIBCRegistry(inputConfigs map[string]*input.ChainInput) ([]registry.ChainIbcData, error) {
	keywords := g.inputLoader.GetRegistryKeywords(inputConfigs)
	log.Printf("Looking for IBC data matching: %v", keywords)

	// Determine cache path
	cachePath := g.config.LocalIbcRegistryPath
	if cachePath == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		cachePath = filepath.Join(currentDir, "ibc-registry")
	}

	// Download registry if not using local data or local data doesn't exist
	if !g.config.UseLocalIbcReg {
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

/*
Fetch the keplr registry from the chainapsis github repository

Params:
- inputConfigs: the input configs

Returns:
- []keplr.KeplrChainConfig: the keplr chain configs
- error: if the keplr registry cannot be fetched
*/
func (g *Generator) fetchKeplrRegistry(inputConfigs map[string]*input.ChainInput) ([]keplr.KeplrChainConfig, error) {
	jsonFileNames, chainsWithoutKeplrJSONFileName := g.inputLoader.GetKeplrJSONFileNames(inputConfigs)
	log.Printf("Looking for Keplr data matching: %v", jsonFileNames)
	if len(chainsWithoutKeplrJSONFileName) > 0 {
		log.Printf("Chains without keplr json file name: %v", chainsWithoutKeplrJSONFileName)
	}

	// Determine cache path
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}
	cachePath := g.config.LocalKeplrRegistryPath
	if cachePath == "" {
		cachePath = filepath.Join(currentDir, "keplr-registry")
	}

	if !g.config.UseLocalKeplrReg {
		if err := os.RemoveAll(cachePath); err != nil {
			return nil, fmt.Errorf("failed to clear keplr cache: %w", err)
		}

		if err := keplr.GetKeplrRegistry(cachePath); err != nil {
			return nil, fmt.Errorf("failed to download Keplr registry: %w", err)
		}
	}

	// Process the keplr registry
	keplrConfigs, err := keplr.ProcessKeplrRegistry(cachePath, jsonFileNames)
	if err != nil {
		return nil, fmt.Errorf("failed to process Keplr registry: %w", err)
	}
	// Append the keplr chain configs that are not in the json file names
	for _, chainID := range chainsWithoutKeplrJSONFileName {
		if inputConfig, exists := inputConfigs[chainID]; exists {
			keplrConfigs = append(keplrConfigs, *inputConfig.Chain.KeplrChainConfig)
		}
	}

	return keplrConfigs, nil
}

// copyChainImages copies all chain images from the images directory to the public directory
func (g *Generator) copyChainImages(inputConfigs map[string]*input.ChainInput, enrichedReg *enriched.RegistryConfig) error {
	// Get absolute path to images directory (relative to current working directory)
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Resolve images directory to absolute path
	imagesDir := filepath.Join(currentDir, "images")
	imagesDir, err = filepath.Abs(imagesDir)
	if err != nil {
		return fmt.Errorf("failed to resolve images directory: %w", err)
	}

	// Ensure images directory exists
	if _, err := os.Stat(imagesDir); os.IsNotExist(err) {
		return fmt.Errorf("images directory does not exist: %s", imagesDir)
	}

	// Resolve public directory to absolute path
	publicDir := g.config.CopyIconsPath
	if !filepath.IsAbs(publicDir) {
		publicDir = filepath.Join(currentDir, publicDir)
	}
	publicDir, err = filepath.Abs(publicDir)
	if err != nil {
		return fmt.Errorf("failed to resolve public directory: %w", err)
	}

	// Ensure public directory exists (or create it)
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		return fmt.Errorf("failed to create public directory: %w", err)
	}

	// Copy images for each chain
	for chainID, chainConfig := range enrichedReg.Chains {
		// Get registry name from the chain config (e.g., "osmosis", "noble")
		registryName := chainConfig.Registry
		if registryName == "" {
			// Fallback: try to get from input config
			if inputConfig, exists := inputConfigs[chainID]; exists {
				registryName = inputConfig.Chain.Registry
			}
			if registryName == "" {
				log.Printf("Warning: no registry name for chain %s, skipping image copy", chainID)
				continue
			}
		}

		if err := cp.CopyChainImages(imagesDir, publicDir, registryName); err != nil {
			return fmt.Errorf("failed to copy images for chain %s: %w", registryName, err)
		}
	}

	return nil
}

func (g *Generator) writePathfinderConfig(config *output.PathfinderConfig) error {
	dir := filepath.Dir(g.config.PathfinderOutputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	format := g.config.PathfinderOutputFormat
	if format == FormatAuto || format == "" {
		format = formatFromExtension(g.config.PathfinderOutputPath)
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
		return fmt.Errorf("failed to marshal pathfinder config: %w", err)
	}

	return os.WriteFile(g.config.PathfinderOutputPath, data, 0644)
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

// GeneratePathfinderOnly generates only the pathfinder configuration.
func (g *Generator) GeneratePathfinderOnly() (*output.PathfinderConfig, error) {
	inputConfigs, err := g.inputLoader.LoadAllConfigs(g.config.InputDir)
	if err != nil {
		return nil, err
	}

	// Fetch IBC registry data
	ibcData, err := g.fetchIBCRegistry(inputConfigs)
	if err != nil {
		return nil, err
	}

	// Fetch Keplr registry data
	keplrConfigs, err := g.fetchKeplrRegistry(inputConfigs)
	if err != nil {
		return nil, err
	}

	enrichedReg, err := g.enrichBuilder.BuildRegistry(inputConfigs, ibcData, keplrConfigs)
	if err != nil {
		return nil, err
	}

	return g.pathfinderConv.Convert(enrichedReg)
}

// GenerateClientOnly generates only the client configuration.
func (g *Generator) GenerateClientOnly() (*output.ClientConfig, error) {
	inputConfigs, err := g.inputLoader.LoadAllConfigs(g.config.InputDir)
	if err != nil {
		return nil, err
	}

	// Fetch IBC registry data
	ibcData, err := g.fetchIBCRegistry(inputConfigs)
	if err != nil {
		return nil, err
	}

	// Fetch Keplr registry data
	keplrConfigs, err := g.fetchKeplrRegistry(inputConfigs)
	if err != nil {
		return nil, err
	}

	enrichedReg, err := g.enrichBuilder.BuildRegistry(inputConfigs, ibcData, keplrConfigs)
	if err != nil {
		return nil, err
	}

	return g.clientConv.Convert(enrichedReg)
}
