package jwtmiddleware

import (
	"fmt"
	"math/big"
	"slices"

	"github.com/DIMO-Network/cloudevent"
	"github.com/DIMO-Network/token-exchange-api/pkg/tokenclaims"
	"github.com/ethereum/go-ethereum/common"
	jwtware "github.com/gofiber/contrib/jwt"
	"github.com/gofiber/fiber/v2"
)

const (
	// TokenClaimsKey is the key for the token claims in the fiber context.
	TokenClaimsKey = "user"
)

// NewJWTMiddleware creates a new JWT token middleware that validates the token and stores the claims in the fiber context.
func NewJWTMiddleware(jwkSetURLs ...string) fiber.Handler {
	return jwtware.New(jwtware.Config{
		JWKSetURLs: jwkSetURLs,
		Claims:     &tokenclaims.Token{},
		ContextKey: TokenClaimsKey,
	})
}

// AllOfPermissions creates a middleware that checks if the token contains all the required.
// This middleware also checks if the token is for the correct contract and token ID.
func AllOfPermissions(contract common.Address, tokenIDParam string, permissions []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tokenID, err := getTokenID(c, tokenIDParam)
		if err != nil {
			return err
		}
		return checkAllPrivileges(c, contract, tokenID, permissions)
	}
}

// OneOfPermissions creates a middleware that checks if the token contains any of the required.
// This middleware also checks if the token is for the correct contract and token ID.
func OneOfPermissions(contract common.Address, tokenIDParam string, permissions []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tokenID, err := getTokenID(c, tokenIDParam)
		if err != nil {
			return err
		}
		return checkOneOfPrivileges(c, contract, tokenID, permissions)
	}
}

// AllOfPermissionsAddress creates a middleware that checks if the token contains all the required.
// This middleware also checks if the token is for the correct contract and token ID.
func AllOfPermissionsAddress(addressParam string, permissions []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ethAddress, err := getEthAddress(c, addressParam)
		if err != nil {
			return err
		}
		return checkAllPrivileges(c, ethAddress, nil, permissions)
	}
}

// OneOfPermissionsAddress creates a middleware that checks if the token contains any of the required.
// This middleware also checks if the token is for the correct contract and token ID.
func OneOfPermissionsAddress(addressParam string, permissions []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ethAddress, err := getEthAddress(c, addressParam)
		if err != nil {
			return err
		}
		return checkOneOfPrivileges(c, ethAddress, nil, permissions)
	}
}

func checkOneOfPrivileges(ctx *fiber.Ctx, contract common.Address, tokenID *big.Int, permissions []string) error {
	claims, err := GetTokenClaim(ctx)
	if err != nil {
		return err
	}
	// This checks that the privileges are for the token specified by the path variable and the contract address is correct.
	err = validateTokenIDAndAddress(ctx, contract, tokenID, claims)
	if err != nil {
		return err
	}

	for _, v := range permissions {
		if slices.Contains(claims.Permissions, v) {
			return ctx.Next()
		}
	}

	return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized! Token does not contain any of the required privileges")
}

func checkAllPrivileges(ctx *fiber.Ctx, contract common.Address, tokenID *big.Int, permissions []string) error {
	claims, err := GetTokenClaim(ctx)
	if err != nil {
		return err
	}
	// This checks that the privileges are for the token specified by the path variable and the contract address is correct.
	err = validateTokenIDAndAddress(ctx, contract, tokenID, claims)
	if err != nil {
		return err
	}

	for _, v := range permissions {
		if !slices.Contains(claims.Permissions, v) {
			return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized! Token does not contain required privileges")
		}
	}

	return ctx.Next()
}

func validateTokenIDAndAddress(ctx *fiber.Ctx, contract common.Address, tokenID *big.Int, claims *tokenclaims.Token) error {
	assetDID, err := cloudevent.DecodeERC721DID(claims.Asset)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized! invalid asset")
	}

	if tokenID != nil && assetDID.TokenID.Cmp(tokenID) != 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized! mismatch token Id provided")
	}
	if assetDID.ContractAddress != contract {
		return fiber.NewError(fiber.StatusUnauthorized, fmt.Sprintf("Provided token is for the wrong contract: %s", assetDID.ContractAddress))
	}
	return nil
}

// GetTokenClaim gets the token claim from the fiber context.
func GetTokenClaim(ctx *fiber.Ctx) (*tokenclaims.Token, error) {
	token, ok := ctx.Locals(TokenClaimsKey).(*tokenclaims.Token)
	if !ok {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Unauthorized! Internal server error while getting token claim")
	}
	return token, nil
}

func getTokenID(c *fiber.Ctx, tokenIDParam string) (*big.Int, error) {
	tokenIDStr := c.Params(tokenIDParam)
	tokenID, ok := big.NewInt(0).SetString(tokenIDStr, 10)
	if !ok {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "Unauthorized! invalid token ID")
	}
	return tokenID, nil
}

func getEthAddress(c *fiber.Ctx, contractParam string) (common.Address, error) {
	contractStr := c.Params(contractParam)
	if !common.IsHexAddress(contractStr) {
		return common.Address{}, fiber.NewError(fiber.StatusUnauthorized, "Unauthorized! invalid contract")
	}
	return common.HexToAddress(contractStr), nil
}
