package toml

type ChainConfig struct {
	Name string `toml:"name"`
	Id string `toml:"id"`
	Type string `toml:"type"`
	Registry string `toml:"registry"`
	ExplorerUrl string `toml:"explorer_url"`
	Slip44 int `toml:"slip44"`
	Chain struct {
		Rest []ChainApis `toml:"rest"`
		RPCs []ChainApis `toml:"rpcs"`
	} `toml:"chain"`
	Tokens []Token `toml:"token"`
}

type ChainApis struct {
	Url string `toml:"url"`
	Provider string `toml:"provider"`
}

type Token struct {
	Denom string `toml:"denom"`
	Name string `toml:"name"`
	Symbol string `toml:"symbol"`
	Exponent int `toml:"exponent"`
	Icon string `toml:"icon"`
}