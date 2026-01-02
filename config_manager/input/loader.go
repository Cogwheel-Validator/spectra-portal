package input

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Loader loads and parses human-readable chain configuration files.
type Loader struct{}

// NewLoader creates a new configuration loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadChainConfig loads a single chain configuration from a TOML file.
func (l *Loader) LoadChainConfig(filePath string) (*ChainInput, error) {
	if !strings.HasSuffix(filePath, ".toml") {
		return nil, fmt.Errorf("config file must be a .toml file: %s", filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filePath, err)
	}

	var config ChainInput
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", filePath, err)
	}

	return &config, nil
}

// LoadAllConfigs loads all chain configurations from a directory.
// Returns a map of chain ID to ChainInput.
func (l *Loader) LoadAllConfigs(dirPath string) (map[string]*ChainInput, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config directory %s: %w", dirPath, err)
	}

	configs := make(map[string]*ChainInput)
	var errs []error

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		config, err := l.LoadChainConfig(filePath)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}

		if config.Chain.ID == "" {
			errs = append(errs, fmt.Errorf("%s: missing chain.id", entry.Name()))
			continue
		}

		if _, exists := configs[config.Chain.ID]; exists {
			errs = append(errs, fmt.Errorf("%s: duplicate chain ID %s", entry.Name(), config.Chain.ID))
			continue
		}

		configs[config.Chain.ID] = config
	}

	if len(errs) > 0 {
		// Log errors but don't fail - allow partial loading
		for _, e := range errs {
			fmt.Printf("Warning: %v\n", e)
		}
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no valid chain configurations found in %s", dirPath)
	}

	return configs, nil
}

// GetRegistryKeywords extracts registry keywords from loaded configs.
// These are used to filter the IBC registry files.
func (l *Loader) GetRegistryKeywords(configs map[string]*ChainInput) []string {
	keywords := make([]string, 0, len(configs))
	for _, config := range configs {
		if config.Chain.Registry != "" {
			keywords = append(keywords, config.Chain.Registry)
		}
	}
	return keywords
}

/*
Extracts keplr json file name from loaded configs.

Params:
- configs: the loaded configs

Returns:
- []string: the keplr json file names
- []string: the chains that do not have a keplr json file name, or have an overwrite keplr chain config
*/
func (l *Loader) GetKeplrJSONFileNames(configs map[string]*ChainInput) ([]string, []string) {
	jsonFileNames := make([]string, 0, len(configs))
	chainsWithoutKeplrJSONFileName := make([]string, 0, len(configs))
	for _, config := range configs {
		// If the keplr json file name is set and not empty and the keplr chain config is not set
		// append json file name. else mark the chain to be processed with the overwrite keplr chain config
		if config.Chain.KeplrJSONFileName != nil && 
		*config.Chain.KeplrJSONFileName != "" && 
		config.Chain.KeplrChainConfig == nil {
			jsonFileNames = append(jsonFileNames, *config.Chain.KeplrJSONFileName)
		} else {
			chainsWithoutKeplrJSONFileName = append(chainsWithoutKeplrJSONFileName, config.Chain.ID)
		}
	}
	return jsonFileNames, chainsWithoutKeplrJSONFileName
}