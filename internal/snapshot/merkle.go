package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
)

// MerkleRoot hashes each datum with sha256 and folds the level pairwise
// (sha256(left||right)), duplicating the last node on odd levels. A single
// leaf is its own root; an empty set has an empty root.
func MerkleRoot(data [][]byte) string {
	if len(data) == 0 {
		return ""
	}
	level := make([][sha256.Size]byte, len(data))
	for i, d := range data {
		level[i] = sha256.Sum256(d)
	}
	const pairSize = 2
	for len(level) > 1 {
		next := make([][sha256.Size]byte, 0, (len(level)+1)/pairSize)
		for i := 0; i < len(level); i += pairSize {
			right := i // an odd tail pairs with itself (last-leaf duplication)
			if i+1 < len(level) {
				right = i + 1
			}
			pair := make([]byte, 0, pairSize*sha256.Size)
			pair = append(pair, level[i][:]...)
			pair = append(pair, level[right][:]...)
			next = append(next, sha256.Sum256(pair))
		}
		level = next
	}
	return hex.EncodeToString(level[0][:])
}
