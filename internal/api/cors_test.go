package api_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/api"
	"github.com/hearthchain/burning-page/internal/bindings"
	"github.com/hearthchain/burning-page/internal/config"
	"github.com/hearthchain/burning-page/internal/journal"
)

const pagesOrigin = "https://genesis.hearth.tech"

// corsServer is a bare server with no artifacts: CORS behavior is independent
// of the data underneath.
func corsServer(t *testing.T, origins []string) *httptest.Server {
	t.Helper()
	j, err := journal.Load("../../data/journal/waves.csv")
	require.NoError(t, err)
	reg, err := bindings.Load(filepath.Join(t.TempDir(), "bindings.jsonl"), 'H')
	require.NoError(t, err)
	var cfg config.Config
	cfg.DataDir = t.TempDir()
	cfg.HearthScheme = "H"
	cfg.AllowedOrigins = origins
	srv := httptest.NewServer(api.New(&fakeNode{}, j, reg, cfg).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func doRequest(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestCORSAllowsListedOrigin(t *testing.T) {
	srv := corsServer(t, []string{pagesOrigin})

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/bind", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", pagesOrigin)
	resp := doRequest(t, req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, pagesOrigin, resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, resp.Header.Values("Vary"), "Origin")
}

func TestCORSIgnoresUnlistedOrigin(t *testing.T) {
	srv := corsServer(t, []string{pagesOrigin})

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/bind", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://evil.example")
	resp := doRequest(t, req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORSDisabledWithoutConfig(t *testing.T) {
	srv := corsServer(t, nil)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/bind", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", pagesOrigin)
	resp := doRequest(t, req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORSWildcardOrigin(t *testing.T) {
	srv := corsServer(t, []string{"*"})

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/bind", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://anywhere.example")
	resp := doRequest(t, req)

	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORSPreflightForBindings(t *testing.T) {
	srv := corsServer(t, []string{pagesOrigin})

	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/api/bindings", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", pagesOrigin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	resp := doRequest(t, req)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, pagesOrigin, resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "Content-Type")
	assert.NotEmpty(t, resp.Header.Get("Access-Control-Max-Age"))
}

func TestCORSPreflightFromUnlistedOrigin(t *testing.T) {
	srv := corsServer(t, []string{pagesOrigin})

	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/api/bindings", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	resp := doRequest(t, req)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORSHeadersOnRealBindingPost(t *testing.T) {
	id := newIdentity(t, "api cors burner")
	srv := newServer(t, id)

	body := `{"source":"` + id.source + `","hearth":"` + id.hearth + `","publicKey":"` + id.pub +
		`","signature":"` + id.sig + `"}`
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/bindings", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", pagesOrigin)
	resp := doRequest(t, req)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, pagesOrigin, resp.Header.Get("Access-Control-Allow-Origin"))
}
