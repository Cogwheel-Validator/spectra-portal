package registry

// Main json struct object from the chain registry
type ChainIbcData struct {
	Schema   string           `json:"$schema"`
	Chain1   IbcChainData     `json:"chain_1"`
	Chain2   IbcChainData     `json:"chain_2"`
	Channels []IbcChannelData `json:"channels"`
}

// IBC chain data
type IbcChainData struct {
	ChainName    string `json:"chain_name"`
	ClientID     string `json:"client_id"`
	ConnectionID string `json:"connection_id"`
}

// IBC channel data
type IbcChannelData struct {
	Chain1   ChannelChainData `json:"chain_1"`
	Chain2   ChannelChainData `json:"chain_2"`
	Ordering string           `json:"ordering"`
	Version  string           `json:"version"`
	Tags     ChannelTags      `json:"tags"`
}

// Channel chain data
type ChannelChainData struct {
	ChannelID string `json:"channel_id"`
	PortID    string `json:"port_id"`
}

// Channel tags
type ChannelTags struct {
	Preferred bool   `json:"preferred"`
	Status    string `json:"status"`
}

/*
The internal Spectra Portal registry

It should hold the information about the chains and channels that are supported by the Spectra Portal.
This should be a part of the final config at least for now.
*/
type IbcRegistry struct {
	Chains []IbcChain `toml:"chains"`
}

/*
IbcChain is a struct that holds the information about a chain that is supported by the Spectra Portal.
*/
type IbcChain struct {
	ChainName string       `toml:"chain_name"`
	ChainId   string       `toml:"chain_id"`
	Channels  []IbcChannel `toml:"channels"`
}

/*
IbcChannel is a struct that holds the information about a channel such as channel data and to which
chain it is connected to.
*/
type IbcChannel struct {
	ChannelId    string `toml:"channel_id"`
	PortId       string `toml:"port_id"`
	ConnectionId string `toml:"connection_id"`
	ToChainName  string `toml:"to_chain_name"`
	ToChainId    string `toml:"to_chain_id"`
}
