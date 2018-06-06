package data

import (
	"crypto/rand"
	mRand "math/rand"
	"testing"
	"time"
)

func TestPieceSigs(t *testing.T) {
	mRand.Seed(time.Now().UnixNano())
	for i := 0; i < 8000; i++ {
		l := mRand.Intn(64 * 1024)
		buff := make([]byte, l)
		rand.Read(buff)
		piece := NewPiece(buff)
		if !piece.VerifySig() {
			t.Fatalf("error verifying signature from original buff: %v", i)
		}
		if !piece.VerifyContent() {
			t.Fatalf("error verifying content from original buff")
		}
	}
}
