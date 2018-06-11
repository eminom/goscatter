package data

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
)

const (
	FileSizeLimit = 300 * 1024 * 1024
	SegmentSize   = 512

	IfDebugFragger = false
)

type Fragger struct {
	size         int
	segCount     int
	chunks       []Piece
	hashSha256   []byte
	originalName string
}

func NewFragger(inpath string) *Fragger {
	stat, err := os.Stat(inpath)
	if err != nil || stat.IsDir() {
		if err != nil {
			log.Fatalf("error: %v", err)
		} else {
			log.Fatalf("error: a directory for %v", inpath)
		}
	}
	if stat.Size() > FileSizeLimit {
		log.Fatalf("file too large")
	}
	size := int(stat.Size())
	segCount := (size + (SegmentSize - 1)) / SegmentSize
	chunks := make([]Piece, segCount)

	fin, err := os.Open(inpath)
	if err != nil {
		log.Fatalf("error opening %v:%v", inpath, err)
	}
	defer fin.Close()
	var buff [SegmentSize]byte

	hMac := sha256.New()
	for i := 0; ; i++ {
		n, err := fin.Read(buff[:]) // see the doc: File.Read
		if err != nil {
			break
		}
		hMac.Write(buff[:n])
		chunks[i] = NewPiece(bytes.Repeat(buff[:n], 1))
	}
	hashSha256 := hMac.Sum(nil)

	if IfDebugFragger {
		log.Printf("loading `%v`...", inpath)
		log.Printf("%v byte(s)", size)
		log.Printf("%v segment(s)", segCount)
		log.Printf("sha256: %v", hex.EncodeToString(hashSha256))
		log.Printf("loading done")
	}

	return &Fragger{
		size:         size,
		segCount:     segCount,
		chunks:       chunks,
		hashSha256:   hashSha256,
		originalName: inpath,
	}
}

func (f *Fragger) GetHash() []byte {
	return f.hashSha256
}

func (f *Fragger) GetSize() int {
	return f.size
}

func (f *Fragger) GetPieces() []Piece {
	return f.chunks
}
