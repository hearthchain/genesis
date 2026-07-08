package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wavesplatform/gowaves/pkg/crypto"
	"github.com/wavesplatform/gowaves/pkg/proto"

	"github.com/hearthchain/burning-page/internal/api"
	"github.com/hearthchain/burning-page/internal/binding"
	"github.com/hearthchain/burning-page/internal/bindings"
	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/config"
	"github.com/hearthchain/burning-page/internal/hearthaddr"
	"github.com/hearthchain/burning-page/internal/journal"
	"github.com/hearthchain/burning-page/internal/store"
)

type fakeNode struct {
	histories map[string][]json.RawMessage
}

func (f *fakeNode) AllTransactions(_ context.Context, addr string) ([]json.RawMessage, error) {
	txs, ok := f.histories[addr]
	if !ok {
		return nil, fmt.Errorf("fake: no such address %s", addr)
	}
	return txs, nil
}

type identity struct {
	source, hearth, pub, sig string
}

func newIdentity(t *testing.T, seed string) identity {
	t.Helper()
	sec, pub, err := crypto.GenerateKeyPair([]byte(seed))
	require.NoError(t, err)
	source, err := proto.NewAddressFromPublicKey(proto.MainNetScheme, pub)
	require.NoError(t, err)
	_, hearthPub, err := crypto.GenerateKeyPair([]byte(seed + " hearth"))
	require.NoError(t, err)
	hearth, err := hearthaddr.New('H', hearthPub)
	require.NoError(t, err)
	sig, err := crypto.Sign(sec, binding.Message(source.String(), hearth))
	require.NoError(t, err)
	return identity{source: source.String(), hearth: hearth, pub: pub.String(), sig: sig.String()}
}

func depositTx(recipient string, amount uint64, heightTS string) json.RawMessage {
	parts := strings.SplitN(heightTS, "@", 2)
	return json.RawMessage(`{"type":4,"id":"dep` + parts[0] + `","sender":"3POther","recipient":"` + recipient +
		`","assetId":null,"amount":` + fmt.Sprint(amount) + `,"fee":100000,"feeAssetId":null,"timestamp":` + parts[1] +
		`,"height":` + parts[0] + `}`)
}

var serverDataDir string

func newServer(t *testing.T, id identity) *httptest.Server {
	t.Helper()
	dataDir := t.TempDir()
	serverDataDir = dataDir

	// One confirmed burn from id.source with a verified transfer history.
	burnTx := `{"type":4,"id":"B1","sender":"` + id.source + `","recipient":"3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1","assetId":null,"amount":100000000000,"fee":100000,"feeAssetId":null,"timestamp":1754049600000,"height":4000010}`
	require.NoError(t, store.AppendJSONL(filepath.Join(dataDir, "burns.jsonl"), map[string]any{
		"txId": "B1", "chain": "waves", "source": id.source, "amountBaseUnits": 100000000000,
		"height": 4000010, "timestamp": "2026-08-01T12:00:00Z", "status": "confirmed",
	}))
	meta := store.TransferMeta{Address: id.source, ReferenceHeight: 4000100, Status: "ok"}
	require.NoError(t, store.WriteTransfers(filepath.Join(dataDir, "transfers", "waves", id.source+".jsonl"), meta,
		[]json.RawMessage{depositTx(id.source, 200000000000, "3000000@1647216000000"), json.RawMessage(burnTx)}))

	j, err := journal.Load("../../data/journal/waves.csv")
	require.NoError(t, err)
	reg, err := bindings.Load(filepath.Join(dataDir, "bindings.jsonl"), 'H')
	require.NoError(t, err)

	var cfg config.Config
	cfg.DataDir = dataDir
	cfg.HearthScheme = "H"
	var cc config.ChainConfig
	cc.Window = chain.Window{Start: 4000000, End: 4001000}
	cfg.Chains = map[string]config.ChainConfig{"waves": cc}
	cfg.AllowedOrigins = []string{"https://genesis.hearth.tech"}

	node := &fakeNode{histories: map[string][]json.RawMessage{
		id.source: {depositTx(id.source, 100000000000, "3000000@1647216000000")},
	}}
	srv := httptest.NewServer(api.New(node, j, reg, cfg).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func postBinding(t *testing.T, url string, id identity, sig string) *http.Response {
	t.Helper()
	body := `{"source":"` + id.source + `","hearth":"` + id.hearth + `","publicKey":"` + id.pub + `","signature":"` + sig + `"}`
	resp, err := http.Post(url+"/api/bindings", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestBindingRoundTripAndAddressBalance(t *testing.T) {
	id := newIdentity(t, "api test burner")
	srv := newServer(t, id)

	resp := postBinding(t, srv.URL, id, id.sig)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	got := getJSON(t, srv.URL+"/api/address/"+id.hearth)
	assert.Equal(t, "49713174000", got["minimumCreditMicro"])
	assert.Equal(t, "49713.174000", got["minimumCredit"])
	burns, ok := got["burns"].([]any)
	require.True(t, ok)
	require.Len(t, burns, 1)
	bindingsList, ok := got["bindings"].([]any)
	require.True(t, ok)
	assert.Equal(t, id.source, bindingsList[0])
}

func TestBindingRejectionsAndValidation(t *testing.T) {
	id := newIdentity(t, "api test burner")
	other := newIdentity(t, "someone else")
	srv := newServer(t, id)

	resp := postBinding(t, srv.URL, id, other.sig)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp2, err := http.Post(srv.URL+"/api/bindings", "application/json", strings.NewReader("{not json"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp2.Body.Close() })
	assert.Equal(t, http.StatusBadRequest, resp2.StatusCode)

	// Unknown but checksum-valid hearth address: 200 with zeros.
	stranger := newIdentity(t, "nobody bound me")
	got := getJSON(t, srv.URL+"/api/address/"+stranger.hearth)
	assert.Equal(t, "0", got["minimumCreditMicro"])

	// Broken checksum: 400.
	respBad, err := http.Get(srv.URL + "/api/address/not-an-address")
	require.NoError(t, err)
	t.Cleanup(func() { _ = respBad.Body.Close() })
	assert.Equal(t, http.StatusBadRequest, respBad.StatusCode)
}

func TestPreviewComputesLayersLive(t *testing.T) {
	id := newIdentity(t, "api test burner")
	srv := newServer(t, id)

	got := getJSON(t, srv.URL+"/api/preview/waves/"+id.source)
	assert.Equal(t, "ok", got["status"])
	layersList, ok := got["layers"].([]any)
	require.True(t, ok)
	require.Len(t, layersList, 1)
	first, ok := layersList[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2022-04-03", first["weekEnd"])
	assert.Equal(t, "49713174000", got["minimumCreditMicro"])

	respBad, err := http.Get(srv.URL + "/api/preview/waves/garbage")
	require.NoError(t, err)
	t.Cleanup(func() { _ = respBad.Body.Close() })
	assert.Equal(t, http.StatusBadRequest, respBad.StatusCode)
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func TestBindPageIsServed(t *testing.T) {
	id := newIdentity(t, "api test burner")
	srv := newServer(t, id)

	resp, err := http.Get(srv.URL + "/bind")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "KeeperWallet", "the page drives the Keeper extension")
	assert.Contains(t, string(body), "hearth-genesis-binding:v1", "the canonical message prefix is baked in")
	assert.Contains(t, string(body), "keeper-v1", "submissions carry the envelope format")
}

func TestKeeperFormatBindingAccepted(t *testing.T) {
	id := newIdentity(t, "api keeper burner")
	srv := newServer(t, id)

	sec, _, err := crypto.GenerateKeyPair([]byte("api keeper burner"))
	require.NoError(t, err)
	sig, err := crypto.Sign(sec, binding.KeeperV1Envelope(binding.Message(id.source, id.hearth)))
	require.NoError(t, err)

	body := `{"source":"` + id.source + `","hearth":"` + id.hearth + `","publicKey":"` + id.pub +
		`","signature":"` + sig.String() + `","format":"keeper-v1"}`
	resp, err := http.Post(srv.URL+"/api/bindings", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// The same envelope signature without the format marker must be rejected.
	rawBody := `{"source":"` + id.source + `","hearth":"` + id.hearth + `","publicKey":"` + id.pub +
		`","signature":"` + sig.String() + `"}`
	resp2, err := http.Post(srv.URL+"/api/bindings", "application/json", strings.NewReader(rawBody))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp2.Body.Close() })
	assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode)
}

func TestAddressShowsPendingBurnsWithoutCredit(t *testing.T) {
	id := newIdentity(t, "api test burner")
	srv := newServer(t, id)
	require.Equal(t, http.StatusCreated, postBinding(t, srv.URL, id, id.sig).StatusCode)

	// A fresh burn recorded by the watcher before maturity.
	require.NoError(t, store.AppendJSONL(filepath.Join(serverDataDir, "burns.jsonl"), map[string]any{
		"txId": "Fresh1", "chain": "waves", "source": id.source, "amountBaseUnits": 10000000,
		"height": 4000900, "timestamp": "2026-08-01T13:00:00Z", "status": "pending_confirmations",
	}))

	got := getJSON(t, srv.URL+"/api/address/"+id.hearth)
	assert.Equal(t, "49713174000", got["minimumCreditMicro"], "pending burns do not add credit")

	burns, ok := got["burns"].([]any)
	require.True(t, ok)
	require.Len(t, burns, 2, "confirmed and pending burns are both visible")
	statuses := map[string]string{}
	for _, raw := range burns {
		b, isMap := raw.(map[string]any)
		require.True(t, isMap)
		txID, _ := b["txId"].(string)
		status, _ := b["status"].(string)
		statuses[txID] = status
	}
	assert.Equal(t, "confirmed", statuses["B1"])
	assert.Equal(t, "pending_confirmations", statuses["Fresh1"])
}
