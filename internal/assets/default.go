//go:build !portable

package assets

// Ensure is a no-op in the default build: the tools come from PATH.
func Ensure() (Tools, error) {
	return Tools{}, nil
}
