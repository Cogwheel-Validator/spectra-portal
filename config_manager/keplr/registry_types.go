package keplr

// The types in this file mirror the chainapsis keplr-chain-registry JSON files, which use camelCase
// keys. They are only used to decode the registry and are converted to ChainConfig right away, so the
// snake_case tags on ChainConfig (used for the client output) stay independent of the registry format.

type registryChainConfig struct {
	RPC                 string                `json:"rpc"`
	Rest                string                `json:"rest"`
	ChainID             string                `json:"chainId"`
	ChainName           string                `json:"chainName"`
	ChainSymbolImageURL string                `json:"chainSymbolImageUrl"`
	Bip44               registryBip44         `json:"bip44"`
	WalletURLForStaking string                `json:"walletUrlForStaking"`
	Bech32Config        registryBech32Config  `json:"bech32Config"`
	Currencies          []registryCurrency    `json:"currencies"`
	FeeCurrencies       []registryFeeCurrency `json:"feeCurrencies"`
	StakeCurrency       registryCurrency      `json:"stakeCurrency"`
	Features            []string              `json:"features"`
}

type registryBip44 struct {
	CoinType int `json:"coinType"`
}

type registryBech32Config struct {
	Bech32PrefixAccAddr  string `json:"bech32PrefixAccAddr"`
	Bech32PrefixAccPub   string `json:"bech32PrefixAccPub"`
	Bech32PrefixValAddr  string `json:"bech32PrefixValAddr"`
	Bech32PrefixValPub   string `json:"bech32PrefixValPub"`
	Bech32PrefixConsAddr string `json:"bech32PrefixConsAddr"`
	Bech32PrefixConsPub  string `json:"bech32PrefixConsPub"`
}

type registryCurrency struct {
	CoinDenom        string `json:"coinDenom"`
	CoinMinimalDenom string `json:"coinMinimalDenom"`
	CoinDecimals     int    `json:"coinDecimals"`
	CoinImageURL     string `json:"coinImageUrl"`
	CoinGeckoID      string `json:"coinGeckoId"`
}

type registryFeeCurrency struct {
	registryCurrency
	GasPriceStep GasPriceStep `json:"gasPriceStep"`
}

func (c registryCurrency) toCurrency() Currency {
	return Currency{
		CoinDenom:        c.CoinDenom,
		CoinMinimalDenom: c.CoinMinimalDenom,
		CoinDecimals:     c.CoinDecimals,
		CoinImageURL:     c.CoinImageURL,
		CoinGeckoID:      c.CoinGeckoID,
	}
}

func (r registryChainConfig) toChainConfig() ChainConfig {
	currencies := make([]Currency, 0, len(r.Currencies))
	for _, c := range r.Currencies {
		currencies = append(currencies, c.toCurrency())
	}

	feeCurrencies := make([]FeeCurrency, 0, len(r.FeeCurrencies))
	for _, c := range r.FeeCurrencies {
		feeCurrencies = append(feeCurrencies, FeeCurrency{
			CoinDenom:        c.CoinDenom,
			CoinMinimalDenom: c.CoinMinimalDenom,
			CoinDecimals:     c.CoinDecimals,
			CoinGeckoID:      c.CoinGeckoID,
			CoinImageURL:     c.CoinImageURL,
			GasPriceStep:     c.GasPriceStep,
		})
	}

	features := r.Features
	if features == nil {
		features = []string{}
	}

	return ChainConfig{
		RPC:                 r.RPC,
		Rest:                r.Rest,
		ChainID:             r.ChainID,
		ChainName:           r.ChainName,
		ChainSymbolImageURL: r.ChainSymbolImageURL,
		Bip44:               Bip44{CoinType: r.Bip44.CoinType},
		WalletURLForStaking: r.WalletURLForStaking,
		Bech32Config: Bech32Config{
			Bech32PrefixAccAddr:  r.Bech32Config.Bech32PrefixAccAddr,
			Bech32PrefixAccPub:   r.Bech32Config.Bech32PrefixAccPub,
			Bech32PrefixValAddr:  r.Bech32Config.Bech32PrefixValAddr,
			Bech32PrefixValPub:   r.Bech32Config.Bech32PrefixValPub,
			Bech32PrefixConsAddr: r.Bech32Config.Bech32PrefixConsAddr,
			Bech32PrefixConsPub:  r.Bech32Config.Bech32PrefixConsPub,
		},
		Currencies:    currencies,
		FeeCurrencies: feeCurrencies,
		StakeCurrency: StakeCurrency(r.StakeCurrency.toCurrency()),
		Features:      features,
	}
}
