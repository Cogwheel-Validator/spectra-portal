package keplr

// ChainConfig is the config for the keplr chain config
// it will be used at 2 places:
//
// 1. To generate files from the keplr chain registry
//
// 2. As a source for the events to overwrite the config that will be within the spectra ibc chain configs
// in case the keplr chain config needs to be overwritten or it doesn't exists in the keplr chain registry
type ChainConfig struct {
	RPC                 string        `json:"rpc" toml:"rpc"`
	Rest                string        `json:"rest" toml:"rest"`
	ChainID             string        `json:"chain_id" toml:"chain_id"`
	ChainName           string        `json:"chain_name" toml:"chain_name"`
	ChainSymbolImageURL string        `json:"chain_symbol_image_url" toml:"chain_symbol_image_url"`
	Bip44               Bip44         `json:"bip44" toml:"bip44"`
	WalletURLForStaking string        `json:"wallet_url_for_staking" toml:"wallet_url_for_staking"`
	Bech32Config        Bech32Config  `json:"bech32_config" toml:"bech32_config"`
	Currencies          []Currency    `json:"currencies" toml:"currencies"`
	FeeCurrencies       []FeeCurrency `json:"fee_currencies" toml:"fee_currencies"`
	StakeCurrency       StakeCurrency `json:"stake_currency" toml:"stake_currency"`
	Features            []string      `json:"features" toml:"features"`
}

// Bip44 holds the BIP44 coin type used to derive accounts for the chain.
type Bip44 struct {
	CoinType int `json:"coin_type" toml:"coin_type"`
}

// Bech32Config holds the bech32 address prefixes for the chain.
type Bech32Config struct {
	Bech32PrefixAccAddr  string `json:"bech32_prefix_acc_addr" toml:"bech32_prefix_acc_addr"`
	Bech32PrefixAccPub   string `json:"bech32_prefix_acc_pub" toml:"bech32_prefix_acc_pub"`
	Bech32PrefixValAddr  string `json:"bech32_prefix_val_addr" toml:"bech32_prefix_val_addr"`
	Bech32PrefixValPub   string `json:"bech32_prefix_val_pub" toml:"bech32_prefix_val_pub"`
	Bech32PrefixConsAddr string `json:"bech32_prefix_cons_addr" toml:"bech32_prefix_cons_addr"`
	Bech32PrefixConsPub  string `json:"bech32_prefix_cons_pub" toml:"bech32_prefix_cons_pub"`
}

// Currency describes a currency supported by the chain.
type Currency struct {
	CoinDenom        string `json:"coin_denom" toml:"coin_denom"`
	CoinMinimalDenom string `json:"coin_minimal_denom" toml:"coin_minimal_denom"`
	CoinDecimals     int    `json:"coin_decimals" toml:"coin_decimals"`
	CoinImageURL     string `json:"coin_image_url" toml:"coin_image_url"`
	CoinGeckoID      string `json:"coin_gecko_id" toml:"coin_gecko_id"`
}

// FeeCurrency describes a currency that can be used to pay transaction fees.
type FeeCurrency struct {
	CoinDenom        string       `json:"coin_denom" toml:"coin_denom"`
	CoinMinimalDenom string       `json:"coin_minimal_denom" toml:"coin_minimal_denom"`
	CoinDecimals     int          `json:"coin_decimals" toml:"coin_decimals"`
	CoinGeckoID      string       `json:"coin_gecko_id" toml:"coin_gecko_id"`
	CoinImageURL     string       `json:"coin_image_url" toml:"coin_image_url"`
	GasPriceStep     GasPriceStep `json:"gas_price_step" toml:"gas_price_step"`
}

// GasPriceStep holds the low, average, and high gas price recommendations.
type GasPriceStep struct {
	Low     float64 `json:"low" toml:"low"`
	Average float64 `json:"average" toml:"average"`
	High    float64 `json:"high" toml:"high"`
}

// StakeCurrency describes the currency used for staking on the chain.
type StakeCurrency struct {
	CoinDenom        string `json:"coin_denom" toml:"coin_denom"`
	CoinMinimalDenom string `json:"coin_minimal_denom" toml:"coin_minimal_denom"`
	CoinDecimals     int    `json:"coin_decimals" toml:"coin_decimals"`
	CoinImageURL     string `json:"coin_image_url" toml:"coin_image_url"`
	CoinGeckoID      string `json:"coin_gecko_id" toml:"coin_gecko_id"`
}
