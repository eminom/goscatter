// go test -test.bench=".*"

package coaputils

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"sync/atomic"
)

type MsgIDGen interface {
	NextID() uint16
}

// new version
type MsgIDGenv2 struct {
	a uint32
}

func NewMsgIDGenv1() MsgIDGen {
	var b [4]byte
	rand.Read(b[:])
	a := binary.BigEndian.Uint32(b[:])
	return &MsgIDGenv2{a: a}
}

func (m *MsgIDGenv2) NextID() uint16 {
	v := atomic.AddUint32(&m.a, 1)
	return uint16(v)
}

////////////////////////////////////
// Old version

type MsgIDGenv0 struct {
	lock *sync.Mutex
	a    uint16
}

func NewMsgIDGenv0() MsgIDGen {
	var b [2]byte
	rand.Read(b[:])
	a := binary.BigEndian.Uint16(b[:])
	return &MsgIDGenv0{
		lock: new(sync.Mutex),
		a:    a,
	}
}

func (m *MsgIDGenv0) NextID() uint16 {
	m.lock.Lock()
	m.a++
	v := m.a
	m.lock.Unlock()
	return v
}
