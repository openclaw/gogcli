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

// DeniedValueKey is the string hashed for one refused value of a denied-values rule:
// the dotted command path, the flag, and the lowercased value after a NUL, so a value
// containing dots cannot collide with another rule's path or flag. Hashing the values
// keeps them out of the binary, like the command rules.
func DeniedValueKey(path, flag, value string) string {
	return path + "." + flag + "\x00" + value
}
