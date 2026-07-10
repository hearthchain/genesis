package eos

import (
	"fmt"
	"regexp"
)

// accountShape is the Antelope account-name alphabet: 1-12 chars of a-z, 1-5
// and interior dots (names cannot start or end with a dot).
var accountShape = regexp.MustCompile(`^[a-z1-5](\.?[a-z1-5]){0,11}$`)

// ValidateAccount rejects strings that are not an EOS mainnet account name.
func ValidateAccount(name string) error {
	if len(name) > 12 || !accountShape.MatchString(name) {
		return fmt.Errorf("eos: not an account name: %q", name)
	}
	return nil
}
