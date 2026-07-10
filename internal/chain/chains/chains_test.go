package chains_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/chain/chains"
	"github.com/hearthchain/genesis/internal/config"
)

func TestDeltasForReplaysWavesHistory(t *testing.T) {
	deltasFor, err := chains.DeltasFor("waves")
	require.NoError(t, err)

	txs := []json.RawMessage{json.RawMessage(
		`{"type":4,"id":"In1","sender":"3POther","recipient":"3PSenderAlice1111111111111111111111","assetId":null,"amount":200000000,"fee":100000,"feeAssetId":null,"timestamp":1753900000000,"height":4000001}`,
	)}
	deltas, status := deltasFor(txs, "3PSenderAlice1111111111111111111111")
	assert.Equal(t, chain.StatusOK, status.Kind)
	require.Len(t, deltas, 1)
	assert.Equal(t, int64(200000000), deltas[0].Amount)
}

func TestDeltasForRejectsUnknownChain(t *testing.T) {
	_, err := chains.DeltasFor("dogecoin")
	assert.ErrorContains(t, err, "dogecoin")
}

func TestBaseUnitsPerChain(t *testing.T) {
	units, err := chains.BaseUnits("waves")
	require.NoError(t, err)
	assert.Equal(t, uint64(100_000_000), units)

	units, err = chains.BaseUnits("eos")
	require.NoError(t, err)
	assert.Equal(t, uint64(10_000), units)

	_, err = chains.BaseUnits("dogecoin")
	assert.ErrorContains(t, err, "dogecoin")
}

func TestDeltasForKnowsEos(t *testing.T) {
	deltasFor, err := chains.DeltasFor("eos")
	require.NoError(t, err)

	row := json.RawMessage(`{"@timestamp":"2026-07-04T16:11:36.000","block_num":508400050,` +
		`"trx_id":"in1","global_sequence":1,"act":{"account":"core.vaulta","name":"transfer",` +
		`"data":{"from":"someoneelse1","to":"alicewyl1235","quantity":"2.0000 A","memo":""}}}`)
	deltas, status := deltasFor([]json.RawMessage{row}, "alicewyl1235")
	assert.Equal(t, chain.StatusOK, status.Kind)
	require.Len(t, deltas, 1)
	assert.Equal(t, int64(20_000), deltas[0].Amount)
}

func TestNewBuildsConfiguredAdapters(t *testing.T) {
	var cc config.ChainConfig
	cc.Nodes.Primary = "https://primary.example"
	cc.Nodes.Secondary = "https://secondary.example"
	cc.BurnAddress = "eosio.null"
	cc.HistoryAPI = "https://hyperion.example"

	a, err := chains.New("eos", cc, 'H')
	require.NoError(t, err)
	assert.Equal(t, "eos", a.Name())

	cc.HistoryAPI = ""
	_, err = chains.New("eos", cc, 'H')
	assert.ErrorContains(t, err, "historyAPI", "eos needs a history index")

	cc.BurnAddress = "3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1"
	w, err := chains.New("waves", cc, 'H')
	require.NoError(t, err)
	assert.Equal(t, "waves", w.Name())
}
