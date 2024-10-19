package random

import "hash/maphash"

// GetRandom return random between 0..max-1.
func GetRandom(max uint64) uint64 {
	return new(maphash.Hash).Sum64() % max
}

// GetRandomInRange return random from a to b.
func GetRandomInRange(a, b uint64) uint64 {
	return GetRandom(b-a) + a
}
