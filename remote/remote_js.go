//go:build js && wasm

package remote

func init() {
	archiveFallback = false
	maxFetches = 5000
	maxKeptBytes = 64 << 20
	maxHeadBytes = 8 << 20
	maxJSONBytes = 32 << 20
}
