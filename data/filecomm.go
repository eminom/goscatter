package data

import (
	"bytes"
	"crypto/sha256"
	"log"
	"os"
)

type FileOpComm struct{}

func (FileOpComm) PiecesToArray(pieces []Piece) [][]byte {
	out := make([][]byte, len(pieces))
	for i := 0; i < len(pieces); i++ {
		out[i] = pieces[i].Chunk
	}
	return out
}

func (FileOpComm) SaveToFile(name string, pieces [][]byte) bool {
	fout, err := os.Create(name)
	if err != nil {
		log.Printf("failed to save to <%v>:%v", name, err)
		return false
	}
	defer fout.Close()
	for _, p := range pieces {
		fout.Write(p)
	}
	return true
}

// verify content directly.
func (FileOpComm) VerifyChunk(pieces [][]byte, hash []byte) bool {
	hmac := sha256.New()
	for _, p := range pieces {
		hmac.Write(p)
	}
	outH := hmac.Sum(nil)
	return bytes.Compare(outH, hash) == 0
}

func (FileOpComm) VerifyFile(name string, hash []byte) bool {
	hmac := sha256.New()
	fin, err := os.Open(name)
	if err != nil {
		log.Printf("verify failed:%v", err)
	}
	defer fin.Close()

	var buf [64 * 1024]byte
	for {
		n, err := fin.Read(buf[:])
		if err != nil {
			break
		}
		hmac.Write(buf[:n])
	}
	outH := hmac.Sum(nil)
	return bytes.Compare(outH, hash) == 0
}
