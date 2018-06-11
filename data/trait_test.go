package data

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestTrait(t *testing.T) {
	var buff [masterHMACSize]byte

	for i := 0; i < 3000; i++ {
		rand.Read(buff[:])
		trait := MakeComposerTrait(buff[:], 1234)
		hash, length, err := ParseComposerTrait(trait)
		if err != nil {
			t.Fatalf("error parsing composer-trait: %v", err)
		}
		if length != 1234 {
			t.Fatalf("error parsing length")
		}
		if 0 != bytes.Compare(buff[:], hash) {
			t.Fatalf("error fetching hash part")
		}
	}
}
