package main

import (
    "crypto/sha256"
    "encoding/binary"
    "encoding/hex"
    "hash"
    "math"
)

// The SHA plumbing of the waypoint stream hash (the canonical
// navpack-verify form: the little endian float64 bits of every
// waypoint in order).

func newHasher() hash.Hash { return sha256.New() }

func putFloat(b []byte, f float64) {
    binary.LittleEndian.PutUint64(b, math.Float64bits(f))
}

func hashHex(h hash.Hash) string {
    return hex.EncodeToString(h.Sum(nil))
}
