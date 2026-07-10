// Package eos implements the chain port for EOS mainnet (rebranded Vaulta):
// burns are plain transfers of the native token A (contract core.vaulta) or
// legacy EOS (contract eosio.token) to the unownable eosio.null account. The
// two tokens are 1:1 fungible through the on-chain core.vaulta swap, so both
// earn credit and histories track the combined balance.
package eos

import (
	"fmt"
	"strconv"
	"strings"
)

// decimals is the fixed precision of both native tokens ("4,A" / "4,EOS").
const decimals = 4

// BaseUnits is how many base units make one whole coin (10^4).
const BaseUnits = 10_000

// ParseQuantity converts an Antelope asset string ("12.3456 A", "1.0000 EOS")
// into base units. Anything but the two native tokens at exactly four
// decimals is rejected: a foreign shape in a transfer row means the row must
// not be interpreted.
func ParseQuantity(quantity string) (uint64, error) {
	value, symbol, ok := strings.Cut(quantity, " ")
	if !ok || (symbol != "A" && symbol != "EOS") {
		return 0, fmt.Errorf("eos: not a native token quantity: %q", quantity)
	}
	whole, frac, ok := strings.Cut(value, ".")
	if !ok || len(frac) != decimals {
		return 0, fmt.Errorf("eos: quantity %q must carry exactly %d decimals", quantity, decimals)
	}
	units, err := strconv.ParseUint(whole+frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("eos: quantity %q: %w", quantity, err)
	}
	return units, nil
}
