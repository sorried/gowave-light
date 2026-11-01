package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"gowave/internal/contracts"
	"gowave/internal/database"
	"gowave/internal/middleware"

	"github.com/golang-jwt/jwt/v5"
)

func TestWarpRegistrationRequiresAdministrator(t *testing.T) {
	e := newProbeEnv(t)
	path := "/api/config-profiles/warp/register"
	if !contracts.AdminOnly(path) {
		t.Fatal("WARP registration must remain administrator-only")
	}
	response, err := http.Post(e.server.URL+path, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous registration: %d", response.StatusCode)
	}
	if status, body := e.do(http.MethodPost, path, map[string]any{}); status != http.StatusBadRequest {
		t.Fatalf("administrator validation: %d %s", status, body)
	}
	if err := e.db.Create(&database.ApiToken{UUID: "warp-token", Name: "all", Scopes: database.StringArray{"*"}, ExpireAt: time.Now().UTC().Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	claims := middleware.TokenClaims{UUID: "warp-token", Role: "API", RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	e.token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(strings.Repeat("s", 32)))
	if err != nil {
		t.Fatal(err)
	}
	if status, body := e.do(http.MethodPost, path, map[string]any{}); status != http.StatusForbidden {
		t.Fatalf("API token registration: %d %s", status, body)
	}
}
