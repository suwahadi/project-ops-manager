package static

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"sync"
)

//go:embed js css img
var FS embed.FS

var (
	versionsMu sync.Mutex
	versions   = map[string]string{}
)

func URL(name string) string {
	versionsMu.Lock()
	defer versionsMu.Unlock()
	v, ok := versions[name]
	if !ok {
		data, err := FS.ReadFile(name)
		if err != nil {
			return "/static/" + name
		}
		sum := sha256.Sum256(data)
		v = hex.EncodeToString(sum[:6])
		versions[name] = v
	}
	return "/static/" + name + "?v=" + v
}
