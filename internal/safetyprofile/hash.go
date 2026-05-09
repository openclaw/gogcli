package safetyprofile

import "hash/fnv"

// HashRule returns the FNV-64a hash of a dotted-path safety rule
// (e.g. "gmail.send"). The build-time generator and the runtime matcher both
// hash with this algorithm so that rule strings never appear in the binary's
// data section.
func HashRule(rule string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(rule))
	return h.Sum64()
}
