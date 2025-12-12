// Command generate runs the config generation pipeline to transform
// human-readable chain configs into generated configs for the solver backend
// and frontend client.
//
// Usage:
//
//	go run ./config_manager/cmd/generate \
//	  --input ./chain_configs \
//	  --solver-output ./generated/solver_config.toml \
//	  --client-output ./generated/client_config.json
package main

import (
	"flag"
	"log"
	"os"
	"strings"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/pipeline"
)

func main() {
	// Define command-line flags
	inputDir := flag.String("input", "./chain_configs", "Directory containing human-readable chain configs")
	solverOutput := flag.String("solver-output", "./generated/solver_config.toml", "Output path for solver config")
	clientOutput := flag.String("client-output", "./generated/client_config.toml", "Output path for client config")
	solverFormat := flag.String("solver-format", "auto", "Solver output format: auto, toml, json")
	clientFormat := flag.String("client-format", "auto", "Client output format: auto, toml, json")
	registryCache := flag.String("registry-cache", "", "Path to cache IBC registry data (optional)")
	chainLogoBase := flag.String("chain-logo-base", "", "Base URL for chain logos")
	skipNetwork := flag.Bool("skip-network", false, "Skip network validation of endpoints")
	useCache := flag.Bool("use-cache", false, "Use cached registry data instead of downloading fresh")
	validate := flag.Bool("validate-only", false, "Only validate configs, don't generate")

	flag.Parse()

	// Validate inputs
	if _, err := os.Stat(*inputDir); os.IsNotExist(err) {
		log.Printf("Error: input directory does not exist: %s", *inputDir)
		os.Exit(1)
	}

	config := pipeline.GeneratorConfig{
		InputDir:              *inputDir,
		SolverOutputPath:      *solverOutput,
		ClientOutputPath:      *clientOutput,
		SolverOutputFormat:    parseFormat(*solverFormat),
		ClientOutputFormat:    parseFormat(*clientFormat),
		RegistryCachePath:     *registryCache,
		ChainLogoBaseURL:      *chainLogoBase,
		SkipNetworkValidation: *skipNetwork,
		UseRegistryCache:      *useCache,
	}

	if *validate {
		config.SolverOutputPath = ""
		config.ClientOutputPath = ""
	}

	generator := pipeline.NewGenerator(config)

	log.Printf("Starting config generation pipeline...")

	result, err := generator.Generate()
	if err != nil {
		log.Printf("Error while generating configs: %v", err)
		os.Exit(1)
	}

	// Print summary
	log.Printf("Summary:")
	log.Printf("Chains processed: %d", result.ChainsProcessed)

	if len(result.Warnings) > 0 {
		log.Printf("Warnings:")
		for _, warning := range result.Warnings {
			log.Printf("\t- %s", warning)
		}
	}

	// Print validation failures
	hasFailures := false
	for chainID, valResult := range result.ValidationResults {
		if !valResult.IsValid {
			hasFailures = true
			log.Printf("%s: validation failed", chainID)
		}
	}

	if hasFailures {
		log.Printf("Some chains failed validation. Check the errors above.")
		os.Exit(1)
	}

	if !*validate {
		log.Printf("Output files:")
		if result.SolverConfigPath != "" {
			log.Printf("\tSolver: %s", result.SolverConfigPath)
		}
		if result.ClientConfigPath != "" {
			log.Printf("\tClient: %s", result.ClientConfigPath)
		}
	}

	log.Printf("Finished the generation pipeline!")
}

func parseFormat(s string) pipeline.OutputFormat {
	switch strings.ToLower(s) {
	case "toml":
		return pipeline.FormatTOML
	case "json":
		return pipeline.FormatJSON
	default:
		return pipeline.FormatAuto
	}
}
