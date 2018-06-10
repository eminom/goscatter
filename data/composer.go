package data

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"log"
	// "time"
	// "github.com/eminom/go-coap"
)

// The hash buff:
// 32 + 4 + 6
// sha256 (full)
// chunk-count 4 bytes
// first 6 bytes of md5sum for the former two

var (
	invalidComposerTrait = errors.New("invalid composer trait")
)

type Composer struct {
	oCh chan<- *WorkItem

	name       string
	hash       []byte // hash for the whole file content
	chunkCount int    // we simply do not store file larger than 2G
	whenDone   func()
}

func NewComposer(name string, trait []byte, oCh chan<- *WorkItem, whenDone func()) *Composer {
	hash, chunkCount, err := ParseComposerTrait(trait)
	if err != nil {
		log.Printf("error: %v", err)
		return nil
	}
	return &Composer{
		name:       name,
		hash:       hash,
		chunkCount: chunkCount,
		whenDone:   whenDone,
	}
}

func (c *Composer) GetName() string {
	return c.name
}

func ParseComposerTrait(trait []byte) ([]byte, int, error) {
	if len(trait) != 32+4+6 {
		return nil, 0, invalidComposerTrait
	}
	hMac := sha256.New()
	hMac.Write(trait[:32+4])
	outR := hMac.Sum(nil)
	if 0 != bytes.Compare(outR[:6], trait[32+4:]) {
		return nil, 0, invalidComposerTrait
	}

	hash := bytes.Repeat(trait[:32], 1)
	chunkCount := int(binary.BigEndian.Uint32(trait[32 : 32+4]))
	return hash, chunkCount, nil
}
