package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	getter "github.com/hashicorp/go-getter"
)

// RegistryGitDownload downloads the IBC registry from the GitHub repository
//
// Params:
//   - dst: the directory to download the registry to
//
// Returns:
//   - error: if the registry cannot be downloaded
//
// Usage:
//   - Used to download the IBC registry from the GitHub repository
func RegistryGitDownload(dst string) error {
	// format for using go getter
	url := "github.com/cosmos/chain-registry//_IBC"
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
	fmt.Printf("Downloading registry from %s to %s", url, dst)
	err := opts.Get()

	if err != nil {
		return fmt.Errorf("failed to download registry: %w", err)
	}
	return nil
}

// ProcessIbcRegistry processes the IBC registry and returns the data
//
// Params:
//   - dst: the directory to read the registry from, this is the directory where the registry is downloaded to
//   - keywords: the keywords to search for, this is the keywords pulled out from chain configurations by
//     looking at the registry string
//
// Returns:
//   - []ChainIbcData: the IBC data
//   - error: if the registry cannot be read or processed
//
// Usage:
//   - Used to process the IBC registry and return the data
func ProcessIbcRegistry(dst string, keywords []string) ([]ChainIbcData, error) {
	ibcData := []ChainIbcData{}
	// read the registry from the file
	files, err := os.ReadDir(dst)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry: %w", err)
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		// check only json files
		// and check if they contain any kind of keyword
		// split them by - in the name and verify if they are in the keyword
		if strings.HasSuffix(name, ".json") {
			name = strings.TrimSuffix(name, ".json")
			chainNames := strings.Split(name, "-")
			if slices.Contains(keywords, chainNames[0]) && slices.Contains(keywords, chainNames[1]) {
				// open the file and read the contents with untrimmed name
				filePath := fmt.Sprintf("%s/%s", dst, file.Name())
				jsonFile, err := os.Open(filePath)
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
				var data ChainIbcData
				err = json.Unmarshal(body, &data)
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal file: %w", err)
				}
				ibcData = append(ibcData, data)
			}
		}

	}
	return ibcData, nil
}
