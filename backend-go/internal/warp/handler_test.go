package warp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testKey(value byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}

func registrationFixture() string {
	return `{"token":"provider-token-must-not-leak","config":{"client_id":"AQID","interface":{"addresses":{"v4":"172.16.0.2","v6":"2606:4700:110:1234::2"}},"peers":[{"public_key":"` + testKey(2) + `","endpoint":{"host":"engage.cloudflareclient.com:2408"}}]}}`
}

func TestRegister(t *testing.T) {
	h := NewHandler()
	h.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != registrationURL || r.Method != http.MethodPost || r.Header.Get("CF-Client-Version") != "a-6.30-3596" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected registration request: %s %s %v", r.Method, r.URL, r.Header)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["key"] != testKey(1) || body["type"] != "PC" || body["model"] != "GoWave" {
			t.Fatal("invalid registration payload")
		}
		if _, err := time.Parse(time.RFC3339Nano, body["tos"]); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(registrationFixture())), Header: make(http.Header)}, nil
	})
	req := httptest.NewRequest(http.MethodPost, "/api/config-profiles/warp/register", strings.NewReader(`{"acceptTerms":true,"publicKey":"`+testKey(1)+`"}`))
	w := httptest.NewRecorder()
	h.Register(w, req)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "provider-token") {
		t.Fatalf("registration: %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Response configuration `json:"response"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Response.PublicKey != testKey(2) || result.Response.Endpoint != "engage.cloudflareclient.com:2408" || len(result.Response.Address) != 2 || result.Response.Address[0] != "172.16.0.2/32" || result.Response.Address[1] != "2606:4700:110:1234::2/128" || result.Response.Reserved != [3]int{1, 2, 3} {
		t.Fatalf("unexpected tunnel configuration: %+v", result.Response)
	}
}

func TestRegisterRejectsInvalidInputBeforeContactingCloudflare(t *testing.T) {
	h := NewHandler()
	h.client.Transport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("invalid input contacted Cloudflare")
		return nil, nil
	})
	for _, body := range []string{`{`, `{}`, `{"publicKey":"` + testKey(1) + `"}`, `{"publicKey":"bad","acceptTerms":true}`, `{"publicKey":"` + testKey(0) + `","acceptTerms":true}`, strings.Repeat("x", 5000)} {
		w := httptest.NewRecorder()
		h.Register(w, httptest.NewRequest(http.MethodPost, "/api/config-profiles/warp/register", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("input %q: %d", body[:min(len(body), 80)], w.Code)
		}
	}
}

func TestRegisterRejectsInvalidCloudflareResponses(t *testing.T) {
	for _, fixture := range []struct {
		status int
		body   string
	}{
		{403, `{"token":"must-not-leak"}`},
		{429, `{"token":"must-not-leak"}`},
		{200, `invalid json`},
		{200, `{}`},
		{200, strings.Replace(registrationFixture(), `"AQID"`, `"AA=="`, 1)},
		{200, strings.Replace(registrationFixture(), "172.16.0.2", "invalid", 1)},
		{200, strings.Replace(registrationFixture(), ":2408", ":0", 1)},
		{200, strings.Replace(registrationFixture(), testKey(2), "bad", 1)},
		{200, `{"padding":"` + strings.Repeat("x", 65<<10) + `"}`},
	} {
		h := NewHandler()
		h.client.Transport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: fixture.status, Body: io.NopCloser(strings.NewReader(fixture.body)), Header: make(http.Header)}, nil
		})
		w := httptest.NewRecorder()
		h.Register(w, httptest.NewRequest(http.MethodPost, "/api/config-profiles/warp/register", strings.NewReader(`{"acceptTerms":true,"publicKey":"`+testKey(1)+`"}`)))
		if w.Code != 502 || strings.Contains(w.Body.String(), "must-not-leak") {
			t.Fatalf("provider response: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestRegisterHonorsCancellation(t *testing.T) {
	h := NewHandler()
	h.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest(http.MethodPost, "/api/config-profiles/warp/register", strings.NewReader(`{"acceptTerms":true,"publicKey":"`+testKey(1)+`"}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	h.Register(w, r)
	if w.Code != 502 {
		t.Fatalf("canceled registration: %d", w.Code)
	}
}
