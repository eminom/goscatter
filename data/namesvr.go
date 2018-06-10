package data

import (
	"errors"
	"log"
)

const (
	maxIdSeqNum = 256
)

var (
	nameServerOverflow = errors.New("name-server overflow")
	noSuchName         = errors.New("this name is not registered")
)

type NameSvr struct {
	nameMap map[int]string
	bitsMap [32]byte
}

func NewNameServer() *NameSvr {
	return &NameSvr{
		nameMap: make(map[int]string),
	}
}

func (n *NameSvr) DeregisterName(name string) error {
	usedID := -1
	for k, v := range n.nameMap {
		if v == name {
			usedID = k
			break
		}
	}
	if usedID < 0 {
		return noSuchName
	}
	delete(n.nameMap, usedID)
	n.freeBit(usedID)
	return nil
}

func (n *NameSvr) IdForName(name string) (int, error) {
	for k, v := range n.nameMap {
		if v == name {
			return k, nil
		}
	}

	freeID := -1
	for i := 0; i < 256; i++ {
		g := i / 8
		off := i % 8
		if 0 == (1<<uint(off))&n.bitsMap[g] {
			freeID = i
			break
		}
	}
	if freeID < 0 {
		log.Printf("name server id full")
		return -1, nameServerOverflow
	}
	n.markBit(freeID)
	n.nameMap[freeID] = name
	return freeID, nil
}

func (n *NameSvr) markBit(id int) {
	g := id / 8
	off := id % 8
	n.bitsMap[g] |= (1 << uint(off))
}

func (n *NameSvr) freeBit(id int) {
	g := id / 8
	off := id % 8
	n.bitsMap[g] &= ^(1 << uint(off))
}
