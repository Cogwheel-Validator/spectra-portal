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
	"fmt"
	"os"
	"strings"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/pipeline"
)

func main() {
	// Define command-line flags
	inputDir := flag.String("input", "./chain_configs", "Directory containing human-readable chain configs")
	solverOutput := flag.String("solver-output", "./generated/solver_config.toml", "Output path for solver config")
	clientOutput := flag.String("client-output", "./generated/client_config.json", "Output path for client config")
	solverFormat := flag.String("solver-format", "auto", "Solver output format: auto, toml, json")
	clientFormat := flag.String("client-format", "auto", "Client output format: auto, toml, json")
	registryCache := flag.String("registry-cache", "", "Path to cache IBC registry data (optional)")
	chainLogoBase := flag.String("chain-logo-base", "", "Base URL for chain logos")
	skipNetwork := flag.Bool("skip-network", false, "Skip network validation of endpoints")
	useCache := flag.Bool("use-cache", false, "Use cached registry data instead of downloading fresh")
	skipDenomQueries := flag.Bool("skip-denom-queries", false, "Skip REST queries for denom traces (use computed hashes)")
	validate := flag.Bool("validate-only", false, "Only validate configs, don't generate")

	flag.Parse()

	// Validate inputs
	if _, err := os.Stat(*inputDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: input directory does not exist: %s\n", *inputDir)
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
		SkipDenomQueries:      *skipDenomQueries,
	}

	if *validate {
		config.SolverOutputPath = ""
		config.ClientOutputPath = ""
	}

	generator := pipeline.NewGenerator(config)

	fmt.Println("Starting config generation pipeline...")
	fmt.Println()

	result, err := generator.Generate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while generating configs: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	fmt.Println("\nSummary:")
	fmt.Printf("Chains processed: %d\n", result.ChainsProcessed)

	if len(result.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, warning := range result.Warnings {
			fmt.Printf("\t- %s\n", warning)
		}
	}

	// Print validation failures
	hasFailures := false
	for chainID, valResult := range result.ValidationResults {
		if !valResult.IsValid {
			hasFailures = true
			fmt.Printf("%s: validation failed\n", chainID)
		}
	}

	if hasFailures {
		fmt.Println("\nSome chains failed validation. Check the errors above.")
		os.Exit(1)
	}

	if !*validate {
		fmt.Println("\nOutput files:")
		if result.SolverConfigPath != "" {
			fmt.Printf("\tSolver: %s\n", result.SolverConfigPath)
		}
		if result.ClientConfigPath != "" {
			fmt.Printf("\tClient: %s\n", result.ClientConfigPath)
		}
	}

	fmt.Println("\nFinished the generation pipeline!")
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
