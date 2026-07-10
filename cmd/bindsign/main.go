// Command bindsign signs a cabinet binding message with a Waves seed phrase.
package main

import (
	"os"

	"github.com/hearthchain/genesis/internal/bindsign"
)

func main() {
	os.Exit(bindsign.Run(os.Args[1:], os.Stdout, os.Stderr))
}
