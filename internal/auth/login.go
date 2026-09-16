package auth

import "errors"

var errNotImplemented = errors.New("auth is not implemented in this version")

// Login will open a real browser for visual sign-in. Reserved for a later
// version. Browser-cookie extraction already provides authenticated sessions.
func Login() error {
	return errNotImplemented
}
