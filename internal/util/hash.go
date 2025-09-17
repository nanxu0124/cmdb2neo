package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// HashMap 返回 map 的稳定 hash，用于内容对比。
func HashMap(m map[string]any) string {
	h := sha256.New()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(fmt.Sprintf("%v", m[k])))
	}
	return hex.EncodeToString(h.Sum(nil))
}
