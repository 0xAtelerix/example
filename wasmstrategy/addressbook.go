package wasmstrategy

import "github.com/0xAtelerix/example/application"

// DefaultAddressBook seeds the runtime with known pools so the example strategy can react.
func DefaultAddressBook() map[string]AddressID {
	return map[string]AddressID{
		normalizeAddress(application.ExampleContractAddress): AddressIDUniswapV2Pair,
		"0x8ad599c3a0ff1de082011efddc58f1908eb6e6d8":         AddressIDUniswapV3Pool, // Uniswap v3 USDC/ETH pool
	}
}
