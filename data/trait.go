package data

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

const (
	masterHMACSize = sha256.Size
	hashTrunc      = 6
)

// The hash buff:
// masterHMACSize + 4 + hashTrunc
// sha256 (full)
// chunk-count 4 bytes
// first `hashTrunc' bytes of sha1 for the former two

var (
	invalidComposerTrait = errors.New("invalid composer trait")
)

func ParseComposerTrait(trait []byte) ([]byte, int, error) {
	if len(trait) != masterHMACSize+4+hashTrunc {
		return nil, 0, invalidComposerTrait
	}
	hMac := sha1.New()
	hMac.Write(trait[:masterHMACSize+4])
	outR := hMac.Sum(nil)
	if 0 != bytes.Compare(outR[:hashTrunc], trait[masterHMACSize+4:]) {
		return nil, 0, invalidComposerTrait
	}

	hash := bytes.Repeat(trait[:masterHMACSize], 1)
	count_ := int(binary.BigEndian.Uint32(trait[masterHMACSize : masterHMACSize+4]))
	return hash, count_, nil
}

func MakeComposerTrait(hash []byte, length int) []byte {
	if len(hash) != masterHMACSize {
		//TODO
		panic("error length of hash")
	}

	var lBuff [4]byte
	binary.BigEndian.PutUint32(lBuff[:], uint32(length))
	hMac := sha1.New()
	hMac.Write(hash)
	hMac.Write(lBuff[:])
	outR := hMac.Sum(nil)

	outBuff := make([]byte, masterHMACSize+4+hashTrunc)
	copy(outBuff, hash)
	copy(outBuff[masterHMACSize:masterHMACSize+4], lBuff[:])
	copy(outBuff[masterHMACSize+4:], outR)
	return outBuff
}
