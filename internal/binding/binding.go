// Package binding defines the canonical cabinet binding message and its
// verification: a source address is bound to a Hearth address by a Curve25519
// signature made with the source address's own key. The signed statement is
// the only authority for attribution; it is published and verifiable offline.
package binding

import (
	"errors"
	"fmt"

	"github.com/wavesplatform/gowaves/pkg/crypto"
	"github.com/wavesplatform/gowaves/pkg/proto"

	"github.com/hearthchain/genesis/internal/hearthaddr"
)

// Errors distinguish the caller-facing failure classes of Verify.
var (
	ErrSourceMismatch = errors.New("binding: public key does not own the source address")
	ErrBadSignature   = errors.New("binding: signature does not verify")
)

// Message returns the canonical bytes a wallet signs to bind source to hearth.
func Message(source, hearth string) []byte {
	return []byte("hearth-genesis-binding:v1:" + source + ":" + hearth)
}

// KeeperV1Envelope wraps the canonical message the way Keeper Wallet's
// signCustomData version 1 does before signing: [255,255,255,1] ++ payload.
// The wallet signs this envelope with the account's own key, so the seed
// phrase never leaves the extension.
func KeeperV1Envelope(msg []byte) []byte {
	return append([]byte{0xff, 0xff, 0xff, 0x01}, msg...)
}

// Verify checks a submitted binding: the hearth address is well-formed under
// hearthScheme, the public key derives exactly the claimed source address on
// Waves mainnet, and the signature covers the canonical message.
func Verify(source, hearth string, hearthScheme byte, publicKeyB58, signatureB58 string) error {
	return verifyOver(Message(source, hearth), source, hearth, hearthScheme, publicKeyB58, signatureB58)
}

// VerifyKeeperV1 checks a binding signed by Keeper Wallet's signCustomData
// version 1: the same canonical message inside the Keeper envelope.
func VerifyKeeperV1(source, hearth string, hearthScheme byte, publicKeyB58, signatureB58 string) error {
	return verifyOver(KeeperV1Envelope(Message(source, hearth)), source, hearth, hearthScheme, publicKeyB58, signatureB58)
}

func verifyOver(signed []byte, source, hearth string, hearthScheme byte, publicKeyB58, signatureB58 string) error {
	if err := hearthaddr.Validate(hearth, hearthScheme); err != nil {
		return err
	}
	pub, err := crypto.NewPublicKeyFromBase58(publicKeyB58)
	if err != nil {
		return fmt.Errorf("binding: bad public key: %w", err)
	}
	derived, err := proto.NewAddressFromPublicKey(proto.MainNetScheme, pub)
	if err != nil {
		return fmt.Errorf("binding: %w", err)
	}
	if derived.String() != source {
		return ErrSourceMismatch
	}
	sig, err := crypto.NewSignatureFromBase58(signatureB58)
	if err != nil {
		return fmt.Errorf("binding: bad signature encoding: %w", err)
	}
	if !crypto.Verify(pub, sig, signed) {
		return ErrBadSignature
	}
	return nil
}
