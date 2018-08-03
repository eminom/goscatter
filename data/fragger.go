package data

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"os"

	"github.com/eminom/godirutil"
)

const (
	FileSizeLimit  = 300 * 1024 * 1024
	IfDebugFragger = false
)

type Fragger struct {
	size         int
	segCount     int
	chunks       []Piece
	hashSha256   []byte
	originalName string
	elSegSize    int
}

func NewFragger(segSize int, inpath string) *Fragger {
	if !IsSpecialDirName(inpath) {
		return newFraggerForFile(segSize, inpath)
	}
	return newFraggerForFileList(segSize)
}

func newFraggerForFile(segSize int, inpath string) *Fragger {
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
	fin, err := os.Open(inpath)
	if err != nil {
		log.Fatalf("error opening %v:%v", inpath, err)
	}
	defer fin.Close()
	return newFraggerFromStream(fin, int(stat.Size()), segSize, inpath)
}

func newFraggerFromStream(instream io.Reader, size int, segmentSize int, prompt string) *Fragger {
	segCount := (size + (segmentSize - 1)) / segmentSize
	chunks := make([]Piece, segCount)

	//var buff [SegmentSize]byte
	buff := make([]byte, segmentSize)

	hMac := sha256.New()
	for i := 0; ; i++ {
		n, err := instream.Read(buff[:]) // see the doc: File.Read
		if err != nil {
			break
		}
		hMac.Write(buff[:n])
		chunks[i] = NewPiece(bytes.Repeat(buff[:n], 1))
	}
	hashSha256 := hMac.Sum(nil)

	if IfDebugFragger {
		log.Printf("loading `%v`...", prompt)
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
		originalName: prompt,
		elSegSize:    segmentSize,
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

func (f *Fragger) GetSegmentSize() int {
	return f.elSegSize
}

func newFraggerForFileList(segSize int) *Fragger {
	ob := bytes.NewBuffer(nil)
	files := dutil.ListFiles(".", dutil.StdIgnorer, dutil.AllFiles)
	for _, file := range files {
		io.WriteString(ob, file)
		io.WriteString(ob, "\n")
	}
	return newFraggerFromStream(ob, ob.Len(), segSize, SpecialDirName)
}
