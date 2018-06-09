package coaputils

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
)

type MsgIDGen struct {
	lock *sync.Mutex
	a    uint16
}

func NewMsgIDGen() *MsgIDGen {
	var b [2]byte
	rand.Read(b[:])
	a := binary.BigEndian.Uint16(b[:])
	return &MsgIDGen{lock: new(sync.Mutex), a: a}
}

func (m *MsgIDGen) NextID() uint16 {
	m.lock.Lock()
	m.a++
	rv := m.a
	m.lock.Unlock()
	return rv
}
