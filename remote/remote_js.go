//go:build js && wasm

package remote

func init() {
	archiveFallback = false
	maxFetches = 5000
}
