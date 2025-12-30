package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// FileReader defines the interface for reading files
type FileReader interface {
	// ReadFile reads the file at the given path and returns the contents
	ReadFile(path string) ([]byte, error)
}

// DefaultFileReader implements FileReader using os.ReadFile
type DefaultFileReader struct{}

func (d *DefaultFileReader) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// RPCPathfinderConfigLoader wraps a FileReader to provide dependency injection for config loading functions
type RPCPathfinderConfigLoader struct {
	fileReader FileReader
}

// NewConfigLoader creates a new ConfigLoader with the given FileReader
func NewRPCPathfinderConfigLoader(fileReader FileReader) *RPCPathfinderConfigLoader {
	return &RPCPathfinderConfigLoader{fileReader: fileReader}
}

// NewDefaultConfigLoader creates a ConfigLoader with the default file reader
func NewDefaultRPCPathfinderConfigLoader() *RPCPathfinderConfigLoader {
	return NewRPCPathfinderConfigLoader(&DefaultFileReader{})
}

// LoadRPCPathfinderConfig loads the RPC pathfinder config from the given path
func (cl *RPCPathfinderConfigLoader) LoadRPCPathfinderConfig(configPath string) (*RPCPathfinderConfig, error) {
	// read the config file
	if !strings.HasSuffix(configPath, ".toml") {
		return nil, fmt.Errorf("config file must be a toml file")
	}
	body, err := cl.fileReader.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// unmarshal the config
	var config RPCPathfinderConfig
	if err := toml.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}
