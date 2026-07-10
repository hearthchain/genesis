// Package bindsign implements the bindsign CLI: it derives a Waves account
// from a seed phrase (standard wallet derivation: account seed digest =
// SecureHash(nonce_be4 || phrase), keys = sha256 of that digest), signs the
// canonical binding message for a Hearth address, and prints a ready curl line
// for POST /api/bindings. It is both the wallet-free binding path and the
// end-to-end verification helper.
package bindsign

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"math"

	"github.com/wavesplatform/gowaves/pkg/crypto"
	"github.com/wavesplatform/gowaves/pkg/proto"

	"github.com/hearthchain/genesis/internal/binding"
	"github.com/hearthchain/genesis/internal/hearthaddr"
)

// Exit codes: exitErr for operational failures, exitUsage for bad invocation.
const (
	exitErr   = 1
	exitUsage = 2
)

// Run executes the CLI with the given args, writing results to stdout and
// diagnostics to stderr. It returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("bindsign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	seed := fs.String("seed", "", "Waves seed phrase (keys derived as wallets do)")
	nonce := fs.Uint("nonce", 0, "account nonce within the seed (wallets start at 0)")
	hearth := fs.String("hearth", "", "Hearth address to bind the source to")
	api := fs.String("api", "http://localhost:8080", "base URL for the curl hint")
	scheme := fs.String("hearth-scheme", "H", "Hearth address scheme byte")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *seed == "" || *hearth == "" || len(*scheme) != 1 || *nonce > math.MaxUint32 {
		sayf(stderr, "bindsign: -seed and -hearth are required, -hearth-scheme must be one byte\n")
		return exitUsage
	}
	if err := hearthaddr.Validate(*hearth, (*scheme)[0]); err != nil {
		sayf(stderr, "bindsign: hearth address: %v\n", err)
		return exitErr
	}

	sec, pub, err := deriveKeys(*seed, uint32(*nonce))
	if err != nil {
		sayf(stderr, "bindsign: %v\n", err)
		return exitErr
	}
	source, err := proto.NewAddressFromPublicKey(proto.MainNetScheme, pub)
	if err != nil {
		sayf(stderr, "bindsign: %v\n", err)
		return exitErr
	}
	sig, err := crypto.Sign(sec, binding.Message(source.String(), *hearth))
	if err != nil {
		sayf(stderr, "bindsign: %v\n", err)
		return exitErr
	}

	sayf(stdout, "source=%s\n", source.String())
	sayf(stdout, "hearth=%s\n", *hearth)
	sayf(stdout, "publicKey=%s\n", pub.String())
	sayf(stdout, "signature=%s\n", sig.String())
	sayf(stdout,
		"curl -X POST %s/api/bindings -H 'Content-Type: application/json' "+
			`-d '{"source":"%s","hearth":"%s","publicKey":"%s","signature":"%s"}'`+"\n",
		*api, source.String(), *hearth, pub.String(), sig.String())
	return 0
}

// deriveKeys mirrors the standard Waves wallet derivation so a binding can be
// signed with the same account a wallet shows for the seed phrase.
func deriveKeys(phrase string, nonce uint32) (crypto.SecretKey, crypto.PublicKey, error) {
	const nonceLen = 4
	prefixed := make([]byte, nonceLen+len(phrase))
	binary.BigEndian.PutUint32(prefixed[:nonceLen], nonce)
	copy(prefixed[nonceLen:], phrase)
	accountSeed, err := crypto.SecureHash(prefixed)
	if err != nil {
		return crypto.SecretKey{}, crypto.PublicKey{}, err
	}
	return crypto.GenerateKeyPair(accountSeed.Bytes())
}

// sayf writes formatted output, deliberately ignoring write errors: the CLI
// has nowhere to report a failing stdout or stderr.
func sayf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}
