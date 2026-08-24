package keplr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/hashicorp/go-getter"
)

// GetKeplrRegistry downloads the keplr registry from the chainapsis github repository
//
// Params:
// - dst: the directory to download the registry to
//
// Returns:
// - error: if the registry cannot be downloaded
//
// Usage:
// - Used to download the keplr registry from the chainapsis github repository
func GetKeplrRegistry(dst string) error {
	url := "github.com/chainapsis/keplr-chain-registry//cosmos"
	deadline := time.Now().Add(120 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	opts := getter.Client{
		Ctx:  ctx,
		Src:  url,
		Dst:  dst,
		Mode: getter.ClientModeDir,
		Detectors: []getter.Detector{
			&getter.GitHubDetector{},
		},
		Getters: map[string]getter.Getter{
			"git": &getter.GitGetter{},
		},
	}
	log.Printf("Downloading keplr registry from %s", url)
	err := opts.Get()
	if err != nil {
		return fmt.Errorf("failed to download keplr registry: %w", err)
	}
	return nil
}

// ProcessKeplrRegistry processes the keplr registry from the chainapsis github repository
//
// Params:
// - dst: the directory to read the registry from
// - jsonNames: the names of the json files to process
//
// Returns:
// - []ChainConfig: the keplr chain configs
// - error: if the registry cannot be processed
//
// Usage:
// - Used to process the keplr registry from the chainapsis github repository
func ProcessKeplrRegistry(dst string, jsonNames []string) ([]ChainConfig, error) {
	keplrConfigs := []ChainConfig{}
	for _, jsonName := range jsonNames {
		filePath := fmt.Sprintf("%s/%s", dst, jsonName)
		jsonFile, err := os.Open(filePath) //nolint:gosec // G304: filePath built from operator-supplied chain config + local registry dir
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer func() {
			if err := jsonFile.Close(); err != nil {
				log.Fatalf("Failed to close file: %v", err)
			}
		}()
		body, err := io.ReadAll(jsonFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		var keplrConfig ChainConfig
		err = json.Unmarshal(body, &keplrConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal file: %w", err)
		}
		keplrConfigs = append(keplrConfigs, keplrConfig)
	}
	return keplrConfigs, nil
}
