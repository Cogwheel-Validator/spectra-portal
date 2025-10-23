package config

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Cogwheel-Validator/spectra-ibc-hub/config_manager/src/models"
	"github.com/spf13/viper"
)

func LoadSolverConfig(configPath string) (*models.SolverConfig, error) {
	// read the config file
	body, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// unmarshal the config
	var config models.SolverConfig
	viper.SetConfigType("toml")
	err = viper.ReadConfig(bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// get the config
	config = models.SolverConfig{
		ChainName: viper.GetString("chain_name"),
		ChainId: viper.GetString("chain_id"),
		ChainType: viper.GetString("chain_type"),
		Routes: viper.Get("routes").([]models.ChainRoute),
	}

	return &config, nil
}