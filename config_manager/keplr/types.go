package keplr

// This KeplrChainConfig is the config for the keplr chain config
// it will be used at 2 places:
//
// 1. To generate files from the keplr chain registry
//
// 2. As a source for the events to overwrite the config that will be within the spectra ibc chain configs
// in case the keplr chain config needs to be overwritten or it doesn't exists in the keplr chain registry
type KeplrChainConfig struct {
	RPC                 string `json:"rpc" toml:"rpc"`
	Rest                string `json:"rest" toml:"rest"`
	ChainID             string `json:"chainId" toml:"chain_id"`
	ChainName           string `json:"chainName" toml:"chain_name"`
	ChainSymbolImageURL string `json:"chainSymbolImageUrl" toml:"chain_symbol_image_url"`
	Bip44               Bip44 `json:"bip44" toml:"bip44"`
	WalletURLForStaking string `json:"walletUrlForStaking" toml:"wallet_url_for_staking"`
	Bech32Config        Bech32Config `json:"bech32Config" toml:"bech32_config"`
	Currencies []Currency `json:"currencies" toml:"currencies"`
	FeeCurrencies []FeeCurrency `json:"feeCurrencies" toml:"fee_currencies"`
	StakeCurrency StakeCurrency `json:"stakeCurrency" toml:"stake_currency"`
	Features []string `json:"features" toml:"features"`
}

type Bip44 struct {
	CoinType int `json:"coinType" toml:"coin_type"`
}

type Bech32Config struct {
	Bech32PrefixAccAddr  string `json:"bech32PrefixAccAddr" toml:"bech32_prefix_acc_addr"`
	Bech32PrefixAccPub   string `json:"bech32PrefixAccPub" toml:"bech32_prefix_acc_pub"`
	Bech32PrefixValAddr  string `json:"bech32PrefixValAddr" toml:"bech32_prefix_val_addr"`
	Bech32PrefixValPub   string `json:"bech32PrefixValPub" toml:"bech32_prefix_val_pub"`
	Bech32PrefixConsAddr string `json:"bech32PrefixConsAddr" toml:"bech32_prefix_cons_addr"`
	Bech32PrefixConsPub  string `json:"bech32PrefixConsPub" toml:"bech32_prefix_cons_pub"`
}

type Currency struct {
	CoinDenom        string `json:"coinDenom" toml:"coin_denom"`
	CoinMinimalDenom string `json:"coinMinimalDenom" toml:"coin_minimal_denom"`
	CoinDecimals     int    `json:"coinDecimals" toml:"coin_decimals"`
	CoinImageURL     string `json:"coinImageUrl" toml:"coin_image_url"`
	CoinGeckoID      string `json:"coinGeckoId" toml:"coin_gecko_id"`
}

type FeeCurrency struct {
	CoinDenom        string `json:"coinDenom" toml:"coin_denom"`
	CoinMinimalDenom string `json:"coinMinimalDenom" toml:"coin_minimal_denom"`
	CoinDecimals     int    `json:"coinDecimals" toml:"coin_decimals"`
	CoinGeckoID      string `json:"coinGeckoId" toml:"coin_gecko_id"`
	CoinImageURL     string `json:"coinImageUrl" toml:"coin_image_url"`
	GasPriceStep     GasPriceStep `json:"gasPriceStep" toml:"gas_price_step"`
}

type GasPriceStep struct {
	Low     float64 `json:"low" toml:"low"`
	Average float64 `json:"average" toml:"average"`
	High    float64 `json:"high" toml:"high"`
}

type StakeCurrency struct {
	CoinDenom        string `json:"coinDenom" toml:"coin_denom"`
	CoinMinimalDenom string `json:"coinMinimalDenom" toml:"coin_minimal_denom"`
	CoinDecimals     int    `json:"coinDecimals" toml:"coin_decimals"`
	CoinImageURL     string `json:"coinImageUrl" toml:"coin_image_url"`
	CoinGeckoID      string `json:"coinGeckoId" toml:"coin_gecko_id"`
}