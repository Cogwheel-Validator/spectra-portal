package models

type AppConfig struct {
	ChainName string `toml:"chain_name"`
	ChainId string `toml:"chain_id"`
	ChainImagePath string `toml:"chain_image_path"`
	ChainExplorerUrl string `toml:"chain_explorer_url"`
	ChainRpcUrls []string `toml:"chain_rpc_url"`
	ChainRestUrls []string `toml:"chain_rest_url"`
	ChainType string `toml:"chain_type"`
	ChainRoutes []ChainRoute `toml:"chain_routes"`
	Tokens []Token `toml:"tokens"`
}

type Token struct {
	Denom string `toml:"denom"`
	Name string `toml:"name"`
	Symbol string `toml:"symbol"`
	Exponent int `toml:"exponent"`
	IconPath string `toml:"icon_path"`
	AllowedChains []string `toml:"allowed_chains"`
}