package warp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"gowave/internal/httpx"
)

const registrationURL = "https://api.cloudflareclient.com/v0a4005/reg"

type Handler struct {
	client *http.Client
}

func NewHandler() *Handler {
	return &Handler{client: &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

type registration struct {
	Config struct {
		ClientID  string `json:"client_id"`
		Interface struct {
			Addresses struct {
				V4 string `json:"v4"`
				V6 string `json:"v6"`
			} `json:"addresses"`
		} `json:"interface"`
		Peers []struct {
			PublicKey string `json:"public_key"`
			Endpoint  struct {
				Host string `json:"host"`
			} `json:"endpoint"`
		} `json:"peers"`
	} `json:"config"`
}

type configuration struct {
	PublicKey string   `json:"publicKey"`
	Endpoint  string   `json:"endpoint"`
	Address   []string `json:"address"`
	Reserved  [3]int   `json:"reserved"`
}

func validKey(value string) bool {
	key, err := base64.StdEncoding.DecodeString(value)
	return err == nil && len(key) == 32 && !bytes.Equal(key, make([]byte, 32))
}

func (r *registration) configuration() (*configuration, error) {
	if len(r.Config.Peers) == 0 || !validKey(r.Config.Peers[0].PublicKey) {
		return nil, errors.New("Cloudflare returned an invalid WireGuard peer")
	}
	peer := r.Config.Peers[0]
	host, port, err := net.SplitHostPort(peer.Endpoint.Host)
	if err != nil || host == "" {
		return nil, errors.New("Cloudflare returned an invalid WireGuard endpoint")
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return nil, errors.New("Cloudflare returned an invalid WireGuard endpoint port")
	}
	reserved, err := base64.StdEncoding.DecodeString(r.Config.ClientID)
	if err != nil || len(reserved) != 3 {
		return nil, errors.New("Cloudflare returned an invalid WireGuard client ID")
	}
	result := &configuration{
		PublicKey: peer.PublicKey, Endpoint: peer.Endpoint.Host,
		Reserved: [3]int{int(reserved[0]), int(reserved[1]), int(reserved[2])},
	}
	for _, value := range []string{r.Config.Interface.Addresses.V4, r.Config.Interface.Addresses.V6} {
		if value == "" {
			continue
		}
		address, err := netip.ParseAddr(value)
		if err != nil || address.IsUnspecified() || address.Zone() != "" {
			return nil, errors.New("Cloudflare returned an invalid WireGuard address")
		}
		result.Address = append(result.Address, netip.PrefixFrom(address, address.BitLen()).String())
	}
	if len(result.Address) == 0 {
		return nil, errors.New("Cloudflare returned no WireGuard addresses")
	}
	return result, nil
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input struct {
		PublicKey   string `json:"publicKey"`
		AcceptTerms bool   `json:"acceptTerms"`
	}
	if err := httpx.Decode(r, 4096, &input); err != nil || !input.AcceptTerms || !validKey(input.PublicKey) {
		httpx.Error(w, r, http.StatusBadRequest, "GW400", "A valid WireGuard public key and acceptance of Cloudflare's terms are required")
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"key": input.PublicKey, "tos": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"type": "PC", "model": "GoWave", "name": "GoWave",
	})
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, registrationURL, bytes.NewReader(payload))
	if err != nil {
		httpx.Error(w, r, http.StatusInternalServerError, httpx.CodeInternal, "Could not create WARP registration request")
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("CF-Client-Version", "a-6.30-3596")
	response, err := h.client.Do(request)
	if err != nil {
		httpx.Error(w, r, http.StatusBadGateway, "GW502", "Could not reach Cloudflare WARP registration")
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		httpx.Error(w, r, http.StatusBadGateway, "GW502", fmt.Sprintf("Cloudflare WARP registration returned HTTP %d", response.StatusCode))
		return
	}
	var registered registration
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&registered); err != nil {
		httpx.Error(w, r, http.StatusBadGateway, "GW502", "Cloudflare returned an invalid WARP registration response")
		return
	}
	result, err := registered.configuration()
	if err != nil {
		httpx.Error(w, r, http.StatusBadGateway, "GW502", err.Error())
		return
	}
	httpx.Response(w, result)
}
