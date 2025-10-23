package models

type SolverConfig struct {
	ChainName string `toml:"chain_name"`
	ChainId string `toml:"chain_id"`
	ChainType string `toml:"chain_type"`
	Routes []ChainRoute `toml:"routes"`
}

type ChainRoute struct {
	ToChain string `toml:"to_chain"`
	ToChainId string `toml:"to_chain_id"`
	ConnectionId string `toml:"connection_id"`
	ChannelId string `toml:"channel_id"`
	PortId string `toml:"port_id"`
	TokenDenom string `toml:"token_denom"`
	TokenExponent int `toml:"token_exponent"`
}