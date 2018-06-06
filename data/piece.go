package data

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/binary"
	"log"
)

// signature

const (
	SHA1_TRUNC_SIZE = 6
)

// Signature length:
// 16 + 2 + 6 = 24
// md5's 16
// uint16 length's 2
// truncated sha1 first's 6
type Piece struct {
	Chunk []byte
	Sig   []byte
}

func makeSigForChunk(chunk []byte) []byte {
	h := md5.New()
	h.Write(chunk)
	md5sig := h.Sum(nil)

	s1 := sha1.New()
	s1.Write(md5sig)

	var len0 [2]byte
	binary.BigEndian.PutUint16(len0[:], uint16(len(chunk)))
	s1.Write(len0[:])

	s1sig := s1.Sum(nil)
	sig := append(md5sig, len0[:]...)
	sig = append(sig, s1sig[:6]...)
	return sig
}

// new piece take ownership of chunk
func NewPiece(chunk []byte) Piece {
	o := bytes.Repeat(chunk, 1)

	return Piece{
		Chunk: o,
		Sig:   makeSigForChunk(o),
	}
}

func (p *Piece) VerifySig() bool {
	if len(p.Sig) != 24 {
		log.Printf("sig length invalid: %v", len(p.Sig))
		return false
	}
	s1 := sha1.New()
	s1.Write(p.Sig[:18])
	s1sig := s1.Sum(nil)

	// log.Printf("sha1: %q", s1sig[:6])
	// log.Printf("last 6:%q", p.Sig[18:])

	return 0 == bytes.Compare(s1sig[:6], p.Sig[18:])
}

func (p *Piece) VerifyContent() bool {
	sig := makeSigForChunk(p.Chunk)
	return 0 == bytes.Compare(p.Sig, sig)
}
