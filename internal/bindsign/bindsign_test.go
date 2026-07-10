package bindsign_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wavesplatform/gowaves/pkg/crypto"

	"github.com/hearthchain/genesis/internal/binding"
	"github.com/hearthchain/genesis/internal/bindsign"
	"github.com/hearthchain/genesis/internal/hearthaddr"
)

func testHearth(t *testing.T) string {
	t.Helper()
	_, pub, err := crypto.GenerateKeyPair([]byte("cabinet destination seed"))
	require.NoError(t, err)
	hearth, err := hearthaddr.New('H', pub)
	require.NoError(t, err)
	return hearth
}

func outputValue(t *testing.T, out, key string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if rest, found := strings.CutPrefix(line, key+"="); found {
			return rest
		}
	}
	t.Fatalf("output has no %s= line:\n%s", key, out)
	return ""
}

func TestRunEmitsBindingThatVerifies(t *testing.T) {
	hearth := testHearth(t)
	var stdout, stderr bytes.Buffer

	code := bindsign.Run([]string{"-seed", "test throwaway seed phrase", "-hearth", hearth}, &stdout, &stderr)
	require.Equal(t, 0, code, "stderr: %s", stderr.String())

	out := stdout.String()
	source := outputValue(t, out, "source")
	pub := outputValue(t, out, "publicKey")
	sig := outputValue(t, out, "signature")

	assert.NoError(t, binding.Verify(source, hearth, 'H', pub, sig))
	assert.Contains(t, out, "curl", "must print a ready-to-run curl line")
}

func TestRunSameSeedDifferentNonceGivesDifferentSource(t *testing.T) {
	hearth := testHearth(t)
	var out0, out1, stderr bytes.Buffer

	require.Equal(t, 0, bindsign.Run([]string{"-seed", "s", "-hearth", hearth}, &out0, &stderr))
	require.Equal(t, 0, bindsign.Run([]string{"-seed", "s", "-nonce", "1", "-hearth", hearth}, &out1, &stderr))

	assert.NotEqual(t, outputValue(t, out0.String(), "source"), outputValue(t, out1.String(), "source"))
}

func TestRunRejectsBadHearthAddress(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := bindsign.Run([]string{"-seed", "s", "-hearth", "not-an-address"}, &stdout, &stderr)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "hearth")
}
