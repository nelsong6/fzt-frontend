package frontend

import (
	"fmt"
	"runtime"

	"github.com/nelsong6/fzt/core"
	"github.com/zalando/go-keyring"
)

const (
	keyringService = "homepage"
	keyringUser    = "jwt-secret"
)

// ReadJWTSecret reads the JWT signing secret from the OS credential store.
// Returns the secret or an error with a user-facing message.
func ReadJWTSecret() (string, error) {
	secret, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		storeNames := map[string]string{
			"windows": "Windows Credential Manager",
			"linux":   "secret-tool / KWallet",
			"darwin":  "macOS Keychain",
		}
		storeName := storeNames[runtime.GOOS]
		return "", fmt.Errorf("JWT secret not found in %s (%s/%s)", storeName, keyringService, keyringUser)
	}
	if secret == "" {
		return "", fmt.Errorf("JWT secret is empty")
	}
	return secret, nil
}

// HandleValidate checks that the JWT secret exists in the current platform's
// credential store. Sets JWTSecret on success, posts a red error on failure.
func HandleValidate(s *core.State) {
	secret, err := ReadJWTSecret()
	if err != nil {
		s.SetTitle(err.Error(), 2)
		return
	}

	s.JWTSecret = secret
	s.SetTitle("credential store OK", 1)
}
