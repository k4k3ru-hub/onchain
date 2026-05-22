//
// evm.go
//
package core

import (
    "fmt"
)


//
// Get EVM chain ID by chain and network.
//
func EVMChainID(chain Chain, network Network) (uint64, error) {
    // Validate.
    if err := chain.Validate(); err != nil {
        return 0, err
    }
    if err := network.Validate(); err != nil {
        return 0, err
    }

    switch chain {
    case ChainEthereum:
        switch network {
        case NetworkMainnet:
            return 1, nil
        case NetworkSepolia:
            return 11155111, nil
        }

    case ChainBase:
        switch network {
        case NetworkMainnet:
            return 8453, nil
        case NetworkSepolia:
            return 84532, nil
        }

    case ChainBNB:
        switch network {
        case NetworkMainnet:
            return 56, nil
        }

    case ChainPolygon:
        switch network {
        case NetworkMainnet:
            return 137, nil
        }

    case ChainAvalanche:
        switch network {
        case NetworkMainnet:
            return 43114, nil
        }
    }

    return 0, fmt.Errorf("unsupported evm network: chain=%q network=%q", chain, network)
}
