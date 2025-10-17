package jwtmiddleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DIMO-Network/token-exchange-api/pkg/tokenclaims"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jose/go-jose/v3"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

const (
	testContract = "0x1234567890123456789012345678901234567890"
	testTokenID  = "12345"
	testAssetDID = "did:erc721:1:0x1234567890123456789012345678901234567890:12345"
)

type mockAuthServer struct {
	server *httptest.Server
	signer jose.Signer
	jwks   jose.JSONWebKey
}

func setupAuthServer(t *testing.T) *mockAuthServer {
	t.Helper()

	// Generate RSA key
	sk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// Generate key ID
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("Failed to generate key ID: %v", err)
	}
	keyID := hex.EncodeToString(b)

	// Create JWK
	jwk := jose.JSONWebKey{
		Key:       sk.Public(),
		KeyID:     keyID,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}

	// Create signer
	sig, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.RS256,
		Key:       sk,
	}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]any{
			"kid": keyID,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	auth := &mockAuthServer{
		signer: sig,
		jwks:   jwk,
	}

	// Create test server with only JWKS endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/keys" {
			http.NotFound(w, r)
			return
		}
		err := json.NewEncoder(w).Encode(jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{jwk},
		})
		if err != nil {
			http.Error(w, "Failed to encode JWKS", http.StatusInternalServerError)
		}
	}))

	auth.server = server
	return auth
}

func (m *mockAuthServer) sign(claim *tokenclaims.Token) (string, error) {
	claim.ExpiresAt = jwt.NewNumericDate(time.Now().Add(1 * time.Hour))
	claim.IssuedAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour))
	claim.Audience = jwt.ClaimStrings{"dimo.zone"}
	claim.Issuer = "http://127.0.0.1:3003"
	b, err := json.Marshal(claim)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}

	out, err := m.signer.Sign(b)
	if err != nil {
		return "", fmt.Errorf("failed to sign claims: %w", err)
	}

	token, err := out.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize token: %w", err)
	}

	return token, nil
}

func (m *mockAuthServer) URL() string {
	return m.server.URL
}

func (m *mockAuthServer) Close() {
	m.server.Close()
}

// setupTestApp creates a new Fiber app for testing with JWT middleware.
func setupTestApp(jwkSetURLs ...string) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).SendString(err.Error())
		},
	})

	// Add JWT middleware if JWK set URLs are provided
	if len(jwkSetURLs) > 0 {
		app.Use(NewJWTMiddleware(jwkSetURLs...))
	}

	return app
}

// setupTokenClaims creates token claims and sets them in the context.
func setupTokenClaims(c *fiber.Ctx, claims *tokenclaims.Token) {
	c.Locals(TokenClaimsKey, claims)
}

// makeToken is a helper function to create a Token with the given asset and permissions.
func makeToken(asset string, permissions []string) *tokenclaims.Token {
	token := &tokenclaims.Token{
		CustomClaims: tokenclaims.CustomClaims{
			Asset:       asset,
			Permissions: permissions,
		},
	}
	return token
}

func TestAllOfPermissions(t *testing.T) {
	contract := common.HexToAddress(testContract)
	authServer := setupAuthServer(t)

	tests := []struct {
		name         string
		tokenIDParam string
		pathValue    string
		permissions  []string
		claims       *tokenclaims.Token
		expectedCode int
	}{
		{
			name:         "all permissions present",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1", "perm2"},
			claims:       makeToken(testAssetDID, []string{"perm1", "perm2", "perm3"}),
			expectedCode: fiber.StatusOK,
		},
		{
			name:         "missing one permission",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1", "perm2", "perm3"},
			claims:       makeToken(testAssetDID, []string{"perm1", "perm2"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "no permissions in token",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "invalid token ID",
			tokenIDParam: "tokenID",
			pathValue:    "invalid",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "empty token ID",
			tokenIDParam: "tokenID",
			pathValue:    "",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusNotFound,
		},
		{
			name:         "negative token ID",
			tokenIDParam: "tokenID",
			pathValue:    "-123",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "mismatched token ID",
			tokenIDParam: "tokenID",
			pathValue:    "99999",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "wrong contract address",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1"},
			claims: makeToken(
				"did:erc721:1:0x0000000000000000000000000000000000000001:12345",
				[]string{"perm1"},
			),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "invalid asset DID",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1"},
			claims:       makeToken("invalid:did:format", []string{"perm1"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "empty required permissions list",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusOK,
		},
		{
			name:         "duplicate permissions",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1", "perm2"},
			claims:       makeToken(testAssetDID, []string{"perm1", "perm1", "perm2"}),
			expectedCode: fiber.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupTestApp() // No JWT middleware for this test
			authRoute := app.Use(NewJWTMiddleware(authServer.URL() + "/keys"))
			// Setup route with middleware
			authRoute.Get(
				fmt.Sprintf("/test/:%s", tt.tokenIDParam),
				AllOfPermissions(contract, tt.tokenIDParam, tt.permissions),
				func(c *fiber.Ctx) error {
					return c.SendStatus(fiber.StatusOK)
				},
			)

			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test/%s", tt.pathValue), nil)
			token, err := authServer.sign(tt.claims)
			require.NoError(t, err)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedCode, resp.StatusCode)
		})
	}
}

func TestOneOfPermissions(t *testing.T) {
	contract := common.HexToAddress(testContract)
	authServer := setupAuthServer(t)

	tests := []struct {
		name         string
		tokenIDParam string
		pathValue    string
		permissions  []string
		claims       *tokenclaims.Token
		expectedCode int
	}{
		{
			name:         "has one of required permissions",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1", "perm2", "perm3"},
			claims:       makeToken(testAssetDID, []string{"perm2"}),
			expectedCode: fiber.StatusOK,
		},
		{
			name:         "has all required permissions",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1", "perm2"},
			claims:       makeToken(testAssetDID, []string{"perm1", "perm2"}),
			expectedCode: fiber.StatusOK,
		},
		{
			name:         "has none of required permissions",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1", "perm2"},
			claims:       makeToken(testAssetDID, []string{"perm3", "perm4"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "no permissions in token",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "invalid token ID",
			tokenIDParam: "tokenID",
			pathValue:    "abc",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "wrong contract for OneOf",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{"perm1"},
			claims: makeToken(
				"did:erc721:1:0x9999999999999999999999999999999999999999:12345",
				[]string{"perm1"},
			),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "empty required permissions list",
			tokenIDParam: "tokenID",
			pathValue:    testTokenID,
			permissions:  []string{},
			claims:       makeToken(testAssetDID, []string{}),
			expectedCode: fiber.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupTestApp() // No JWT middleware for this test
			authRoute := app.Use(NewJWTMiddleware(authServer.URL() + "/keys"))
			// Setup route with middleware
			authRoute.Get(
				fmt.Sprintf("/test/:%s", tt.tokenIDParam),
				OneOfPermissions(contract, tt.tokenIDParam, tt.permissions),
				func(c *fiber.Ctx) error {
					return c.SendStatus(fiber.StatusOK)
				},
			)

			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test/%s", tt.pathValue), nil)
			token, err := authServer.sign(tt.claims)
			require.NoError(t, err)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedCode, resp.StatusCode)
		})
	}
}

func TestAllOfPermissionsAddress(t *testing.T) {
	authServer := setupAuthServer(t)

	tests := []struct {
		name         string
		addressParam string
		pathValue    string
		permissions  []string
		claims       *tokenclaims.Token
		expectedCode int
	}{
		{
			name:         "all permissions present with valid address",
			addressParam: "address",
			pathValue:    testContract,
			permissions:  []string{"perm1", "perm2"},
			claims:       makeToken(testAssetDID, []string{"perm1", "perm2", "perm3"}),
			expectedCode: fiber.StatusOK,
		},
		{
			name:         "missing one permission with address",
			addressParam: "address",
			pathValue:    testContract,
			permissions:  []string{"perm1", "perm2"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "invalid ethereum address",
			addressParam: "address",
			pathValue:    "invalid_address",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "empty address",
			addressParam: "address",
			pathValue:    "",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusNotFound,
		},
		{
			name:         "short hex address",
			addressParam: "address",
			pathValue:    "0x123",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "address without 0x prefix is accepted by IsHexAddress",
			addressParam: "address",
			pathValue:    "1234567890123456789012345678901234567890",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusOK,
		},
		{
			name:         "mismatched address",
			addressParam: "address",
			pathValue:    "0x0000000000000000000000000000000000000001",
			permissions:  []string{"perm1"},
			claims: makeToken(
				testAssetDID,
				[]string{"perm1"},
			),
			expectedCode: fiber.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupTestApp() // No JWT middleware for this test
			authRoute := app.Use(NewJWTMiddleware(authServer.URL() + "/keys"))
			// Setup route with middleware
			authRoute.Get(
				fmt.Sprintf("/test/:%s", tt.addressParam),
				AllOfPermissionsAddress(tt.addressParam, tt.permissions),
				func(c *fiber.Ctx) error {
					return c.SendStatus(fiber.StatusOK)
				},
			)

			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test/%s", tt.pathValue), nil)
			token, err := authServer.sign(tt.claims)
			require.NoError(t, err)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedCode, resp.StatusCode)
		})
	}
}

func TestOneOfPermissionsAddress(t *testing.T) {
	authServer := setupAuthServer(t)

	tests := []struct {
		name         string
		addressParam string
		pathValue    string
		permissions  []string
		claims       *tokenclaims.Token
		expectedCode int
	}{
		{
			name:         "has one permission with valid address",
			addressParam: "address",
			pathValue:    testContract,
			permissions:  []string{"perm1", "perm2"},
			claims:       makeToken(testAssetDID, []string{"perm2"}),
			expectedCode: fiber.StatusOK,
		},
		{
			name:         "has none of required permissions",
			addressParam: "address",
			pathValue:    testContract,
			permissions:  []string{"perm1", "perm2"},
			claims:       makeToken(testAssetDID, []string{"perm3"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "invalid address format",
			addressParam: "address",
			pathValue:    "not_an_address",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "address too long",
			addressParam: "address",
			pathValue:    "0x12345678901234567890123456789012345678901234",
			permissions:  []string{"perm1"},
			claims:       makeToken(testAssetDID, []string{"perm1"}),
			expectedCode: fiber.StatusUnauthorized,
		},
		{
			name:         "has multiple matching permissions",
			addressParam: "address",
			pathValue:    testContract,
			permissions:  []string{"perm1", "perm2", "perm3"},
			claims:       makeToken(testAssetDID, []string{"perm1", "perm2"}),
			expectedCode: fiber.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupTestApp() // No JWT middleware for this test
			authRoute := app.Use(NewJWTMiddleware(authServer.URL() + "/keys"))
			// Setup route with middleware
			authRoute.Get(
				fmt.Sprintf("/test/:%s", tt.addressParam),
				OneOfPermissionsAddress(tt.addressParam, tt.permissions),
				func(c *fiber.Ctx) error {
					return c.SendStatus(fiber.StatusOK)
				},
			)

			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test/%s", tt.pathValue), nil)
			token, err := authServer.sign(tt.claims)
			require.NoError(t, err)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedCode, resp.StatusCode)
		})
	}
}
