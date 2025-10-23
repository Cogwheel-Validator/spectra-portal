package toml

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// validator is a function type that validates a specific aspect of the ChainConfig
type validator func(config *ChainConfig, fileName string) error

// validateName checks if the chain name is provided
func validateName(config *ChainConfig, fileName string) error {
	if config.Name == "" {
		return fmt.Errorf("chain name is required in %s", fileName)
	}
	return nil
}

// validateID checks if the chain ID is provided
func validateID(config *ChainConfig, fileName string) error {
	if config.Id == "" {
		return fmt.Errorf("chain id is required in %s", fileName)
	}
	return nil
}

// validateType checks if the chain type is provided and supported
func validateType(config *ChainConfig, fileName string) error {
	if config.Type == "" {
		return fmt.Errorf("chain type is required in %s", fileName)
	}
	if !slices.Contains(ChainTypes, config.Type) {
		return fmt.Errorf("chain type %s is not supported in %s", config.Type, fileName)
	}
	return nil
}

// validateRegistry checks if the chain registry is provided
func validateRegistry(config *ChainConfig, fileName string) error {
	if config.Registry == "" {
		return fmt.Errorf("chain registry is required in %s", fileName)
	}
	return nil
}

// validateExplorerUrl checks if the explorer URL is provided and reachable
func validateExplorerUrl(config *ChainConfig, fileName string) error {
	if config.ExplorerUrl == "" {
		return fmt.Errorf("chain explorer url is required in %s", fileName)
	}
	// do a fetch to the explorer url to verify if it is a valid url
	// we just need to check if the url is reachable
	_, err := http.Head(config.ExplorerUrl)
	if err != nil {
		return fmt.Errorf("failed to check if explorer url is reachable in %s: %w", fileName, err)
	}
	return nil
}

// validateRPCs checks if at least one RPC endpoint is provided
func validateRPCs(config *ChainConfig, fileName string) error {
	if len(config.Chain.RPCs) == 0 {
		return fmt.Errorf("chain rpcs are required in %s", fileName)
	}
	return nil
}

// validateRest checks if at least one REST endpoint is provided
func validateRest(config *ChainConfig, fileName string) error {
	if len(config.Chain.Rest) == 0 {
		return fmt.Errorf("chain rest are required in %s", fileName)
	}
	return nil
}

// Validate runs all validators on the ChainConfig and returns any errors found
func (c *ChainConfig) Validate(fileName string) error {
	validators := []validator{
		validateName,
		validateID,
		validateType,
		validateRegistry,
		validateExplorerUrl,
		validateRPCs,
		validateRest,
	}

	var validationErrors []error
	for _, validate := range validators {
		if err := validate(c, fileName); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if len(validationErrors) > 0 {
		return errors.Join(validationErrors...)
	}
	return nil
}

// ValidateHumanConfig validates the chain configurations written by humans in the chain_configs directory
func ValidateHumanConfig(src string) error {
	// read all of the chain configurations written by devs in the chain_configs directory
	files, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read chain configurations: %w", err)
	}

	var errorMessages []error
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if strings.HasSuffix(file.Name(), ".toml") {
			filePath := fmt.Sprintf("%s/%s", src, file.Name())
			body, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read chain configuration: %w", err)
			}

			var chainConfig ChainConfig
			err = toml.Unmarshal(body, &chainConfig)
			if err != nil {
				err = fmt.Errorf("failed to unmarshal chain configuration %s: %w", file.Name(), err)
				errorMessages = append(errorMessages, err)
				continue
			}

			// validate the chain configuration
			if err := chainConfig.Validate(file.Name()); err != nil {
				errorMessages = append(errorMessages, err)
			}
		}
	}

	if len(errorMessages) > 0 {
		return errors.Join(errorMessages...)
	}
	return nil
}