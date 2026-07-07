package snapshot_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wavesplatform/gowaves/pkg/crypto"
	"github.com/wavesplatform/gowaves/pkg/proto"

	"github.com/hearthchain/burning-page/internal/binding"
	"github.com/hearthchain/burning-page/internal/bindings"
	"github.com/hearthchain/burning-page/internal/hearthaddr"
	"github.com/hearthchain/burning-page/internal/journal"
	"github.com/hearthchain/burning-page/internal/snapshot"
	"github.com/hearthchain/burning-page/internal/store"
)

func loadJournal(t *testing.T) *journal.Journal {
	t.Helper()
	j, err := journal.Load("../../data/journal/waves.csv")
	require.NoError(t, err)
	return j
}

// seedIdentity derives a real Waves source address and a bound Hearth address.
func seedIdentity(t *testing.T, seed string) (source, hearth, pub, sig string) {
	t.Helper()
	sec, pubKey, err := crypto.GenerateKeyPair([]byte(seed))
	require.NoError(t, err)
	addr, err := proto.NewAddressFromPublicKey(proto.MainNetScheme, pubKey)
	require.NoError(t, err)
	_, hearthPub, err := crypto.GenerateKeyPair([]byte(seed + " hearth"))
	require.NoError(t, err)
	h, err := hearthaddr.New('H', hearthPub)
	require.NoError(t, err)
	s, err := crypto.Sign(sec, binding.Message(addr.String(), h))
	require.NoError(t, err)
	return addr.String(), h, pubKey.String(), s.String()
}

// writeArtifacts lays down a minimal data dir: one bound burner (the spec
// example: 1000 WAVES held since March 2022) and one unbound burner.
func writeArtifacts(t *testing.T) (dataDir, hearth, strangerSource string) {
	t.Helper()
	dataDir = t.TempDir()
	source, h, pub, sig := seedIdentity(t, "snapshot bound burner")
	stranger, _, _, _ := seedIdentity(t, "snapshot stranger")

	burn := func(id, src string, amount uint64, height uint64) map[string]any {
		return map[string]any{
			"txId": id, "chain": "waves", "source": src, "amountWavelets": amount,
			"height": height, "timestamp": "2026-08-01T12:00:00Z", "status": "confirmed",
		}
	}
	require.NoError(t, store.AppendJSONL(filepath.Join(dataDir, "burns.jsonl"), burn("B1", source, 100_000_000_000, 4000010)))
	require.NoError(t, store.AppendJSONL(filepath.Join(dataDir, "burns.jsonl"), burn("X1", stranger, 5_000_000_000, 4000020)))

	writeHistory := func(src, burnID string, deposit, burnAmount uint64) {
		depositTx := `{"type":4,"id":"dep-` + burnID + `","sender":"3POther","recipient":"` + src + `","assetId":null,"amount":` +
			itoa(deposit) + `,"fee":100000,"feeAssetId":null,"timestamp":1647216000000,"height":3000000}`
		burnTx := `{"type":4,"id":"` + burnID + `","sender":"` + src + `","recipient":"3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1","assetId":null,"amount":` +
			itoa(burnAmount) + `,"fee":100000,"feeAssetId":null,"timestamp":1754049600000,"height":4000010}`
		meta := store.TransferMeta{Address: src, ReferenceHeight: 4000100, Status: "ok"}
		require.NoError(t, store.WriteTransfers(
			filepath.Join(dataDir, "transfers", src+".jsonl"), meta,
			[]jsonRaw{jsonRaw(depositTx), jsonRaw(burnTx)}))
	}
	writeHistory(source, "B1", 200_000_000_000, 100_000_000_000)
	writeHistory(stranger, "X1", 10_000_000_000, 5_000_000_000)

	reg, err := bindings.Load(filepath.Join(dataDir, "bindings.jsonl"), 'H')
	require.NoError(t, err)
	require.NoError(t, reg.Add(bindings.Record{Source: source, Chain: "waves", Hearth: h, PublicKey: pub, Signature: sig}))

	return dataDir, h, stranger
}

func TestBuildAggregatesCreditsAndSeparatesPending(t *testing.T) {
	dataDir, hearth, stranger := writeArtifacts(t)

	snap, bundles, err := snapshot.Build(dataDir, loadJournal(t), 'H')
	require.NoError(t, err)

	require.Len(t, snap.Entries, 1)
	assert.Equal(t, hearth, snap.Entries[0].Hearth)
	assert.Equal(t, "49713174000", snap.Entries[0].CreditMicro, "the spec example: 49713.174 HRTH")
	assert.Equal(t, []string{stranger}, snap.PendingSources, "confirmed burn without a binding waits")
	assert.NotEmpty(t, snap.Root)
	assert.Equal(t, "49713174000", snap.TotalCreditMicro, "pending credits are not minted")

	require.Len(t, bundles, 2)
	byTx := map[string]string{}
	for _, b := range bundles {
		byTx[b.TxID] = b.Hearth
	}
	assert.Equal(t, hearth, byTx["B1"])
	assert.Empty(t, byTx["X1"])
}

func TestWriteThenVerifyRoundTripsAndDetectsTampering(t *testing.T) {
	dataDir, _, _ := writeArtifacts(t)
	j := loadJournal(t)

	snap, bundles, err := snapshot.Build(dataDir, j, 'H')
	require.NoError(t, err)
	require.NoError(t, snapshot.Write(dataDir, snap, bundles))

	require.NoError(t, snapshot.Verify(dataDir, j, 'H'))

	// Two consecutive writes are byte-identical.
	before, err := os.ReadFile(filepath.Join(dataDir, "snapshot.json"))
	require.NoError(t, err)
	require.NoError(t, snapshot.Write(dataDir, snap, bundles))
	after, err := os.ReadFile(filepath.Join(dataDir, "snapshot.json"))
	require.NoError(t, err)
	assert.Equal(t, before, after)

	// Tampering with the stored root must fail verification.
	tampered := []byte(string(before))
	copy(tampered[len(tampered)/2:], []byte("00"))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "snapshot.json"), tampered, 0o600))
	assert.Error(t, snapshot.Verify(dataDir, j, 'H'))
}

func TestMerkleRootPinsConstruction(t *testing.T) {
	leafA := sha256.Sum256([]byte("a"))
	leafB := sha256.Sum256([]byte("b"))

	single := snapshot.MerkleRoot([][]byte{[]byte("a")})
	assert.Equal(t, hex.EncodeToString(leafA[:]), single, "one leaf is its own root")

	pair := sha256.Sum256(append(leafA[:], leafB[:]...))
	double := snapshot.MerkleRoot([][]byte{[]byte("a"), []byte("b")})
	assert.Equal(t, hex.EncodeToString(pair[:]), double)

	// Odd leaf count duplicates the last leaf.
	third := sha256.Sum256([]byte("c"))
	rightPair := sha256.Sum256(append(third[:], third[:]...))
	expectedRoot := sha256.Sum256(append(pair[:], rightPair[:]...))
	triple := snapshot.MerkleRoot([][]byte{[]byte("a"), []byte("b"), []byte("c")})
	assert.Equal(t, hex.EncodeToString(expectedRoot[:]), triple)
}

type jsonRaw = json.RawMessage

func itoa(v uint64) string {
	return strconv.FormatUint(v, 10)
}
