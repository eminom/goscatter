package coaputils

import (
	"testing"
)

func BenchmarkMsgGenv0(b *testing.B) {
	m := NewMsgIDGenv0()
	for i := 0; i < b.N; i++ {
		m.NextID()
	}
}

func BenchmarkMsgGenv1(b *testing.B) {
	m := NewMsgIDGenv1()
	for i := 0; i < b.N; i++ {
		m.NextID()
	}
}

func TestMsgGen(t *testing.T) {
	m := NewMsgIDGenv0()
	t.Logf("%v", m.NextID())
}
