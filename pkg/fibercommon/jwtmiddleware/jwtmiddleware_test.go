package jwtmiddleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DIMO-Network/token-exchange-api/pkg/tokenclaims"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

const (
	testContract = "0x1234567890123456789012345678901234567890"
	testTokenID  = "12345"
	testAssetDID = "did:erc721:1:0x1234567890123456789012345678901234567890:12345"
)

// setupTestApp creates a new Fiber app for testing.
func setupTestApp() *fiber.App {
	return fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).SendString(err.Error())
		},
	})
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

func TestGetTokenClaim(t *testing.T) {
	tests := []struct {
		name          string
		setupClaims   func(*fiber.Ctx)
		expectedError bool
		expectedCode  int
	}{
		{
			name: "valid token claims",
			setupClaims: func(c *fiber.Ctx) {
				setupTokenClaims(c, makeToken(testAssetDID, []string{"perm1", "perm2"}))
			},
			expectedError: false,
		},
		{
			name: "missing token claims in context",
			setupClaims: func(c *fiber.Ctx) {
				// Don't set any claims
			},
			expectedError: true,
			expectedCode:  fiber.StatusUnauthorized,
		},
		{
			name: "invalid type in context",
			setupClaims: func(c *fiber.Ctx) {
				c.Locals(TokenClaimsKey, "invalid_type")
			},
			expectedError: true,
			expectedCode:  fiber.StatusUnauthorized,
		},
		{
			name: "nil value in context",
			setupClaims: func(c *fiber.Ctx) {
				c.Locals(TokenClaimsKey, nil)
			},
			expectedError: true,
			expectedCode:  fiber.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := setupTestApp()

			app.Get("/test", func(c *fiber.Ctx) error {
				tt.setupClaims(c)
				claims, err := GetTokenClaim(c)

				if tt.expectedError {
					require.Error(t, err)
					if e, ok := err.(*fiber.Error); ok {
						require.Equal(t, tt.expectedCode, e.Code)
					}
					return err
				}

				require.NoError(t, err)
				require.NotNil(t, claims)
				return c.SendStatus(fiber.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close() //nolint:errcheck

			if tt.expectedError {
				require.Equal(t, tt.expectedCode, resp.StatusCode)
			} else {
				require.Equal(t, fiber.StatusOK, resp.StatusCode)
			}
		})
	}
}

func TestAllOfPermissions(t *testing.T) {
	contract := common.HexToAddress(testContract)

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
			app := setupTestApp()

			// Setup route with middleware
			app.Get(
				fmt.Sprintf("/test/:%s", tt.tokenIDParam),
				func(c *fiber.Ctx) error {
					setupTokenClaims(c, tt.claims)
					return c.Next()
				},
				AllOfPermissions(contract, tt.tokenIDParam, tt.permissions),
				func(c *fiber.Ctx) error {
					return c.SendStatus(fiber.StatusOK)
				},
			)

			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test/%s", tt.pathValue), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedCode, resp.StatusCode)
		})
	}
}

func TestOneOfPermissions(t *testing.T) {
	contract := common.HexToAddress(testContract)

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
			app := setupTestApp()

			app.Get(
				fmt.Sprintf("/test/:%s", tt.tokenIDParam),
				func(c *fiber.Ctx) error {
					setupTokenClaims(c, tt.claims)
					return c.Next()
				},
				OneOfPermissions(contract, tt.tokenIDParam, tt.permissions),
				func(c *fiber.Ctx) error {
					return c.SendStatus(fiber.StatusOK)
				},
			)

			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test/%s", tt.pathValue), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedCode, resp.StatusCode)

		})
	}
}

func TestAllOfPermissionsAddress(t *testing.T) {
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
			app := setupTestApp()

			app.Get(
				fmt.Sprintf("/test/:%s", tt.addressParam),
				func(c *fiber.Ctx) error {
					setupTokenClaims(c, tt.claims)
					return c.Next()
				},
				AllOfPermissionsAddress(tt.addressParam, tt.permissions),
				func(c *fiber.Ctx) error {
					return c.SendStatus(fiber.StatusOK)
				},
			)

			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test/%s", tt.pathValue), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedCode, resp.StatusCode)
		})
	}
}

func TestOneOfPermissionsAddress(t *testing.T) {
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
			app := setupTestApp()

			app.Get(
				fmt.Sprintf("/test/:%s", tt.addressParam),
				func(c *fiber.Ctx) error {
					setupTokenClaims(c, tt.claims)
					return c.Next()
				},
				OneOfPermissionsAddress(tt.addressParam, tt.permissions),
				func(c *fiber.Ctx) error {
					return c.SendStatus(fiber.StatusOK)
				},
			)

			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test/%s", tt.pathValue), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedCode, resp.StatusCode)

		})
	}
}
