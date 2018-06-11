package data

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func TestNameServer(t *testing.T) {
	ns := NewNameServer()

	var buf [40]byte
	for i := 0; i < 256; i++ {
		rand.Read(buf[:])
		name := hex.EncodeToString(buf[:])
		_, err := ns.IdForName(name)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	}

	rand.Read(buf[:])
	_, err := ns.IdForName(hex.EncodeToString(buf[:]))
	if err != nameServerOverflow {
		t.Fatalf("not as expected: %v", err)
	}

}

func TestNameServerDos(t *testing.T) {
	ns := NewNameServer()
	id, err := ns.IdForName("hey")
	if 0 != id {
		t.Fatalf("not as expected")
	}
	if err != nil {
		t.Fatalf("error:%v", err)
	}

	id0, err := ns.IdForName("eminem")
	if err != nil {
		t.Fatalf("error")
	}
	id1, err := ns.IdForName("eminem")
	if err != nil {
		t.Fatalf("error")
	}
	if id0 != id1 {
		t.Fatalf("error")
	}

	err = ns.DeregisterName("hey")
	if err != nil {
		t.Fatalf("error:%v", err)
	}

	err = ns.DeregisterName("hey")
	if err != noSuchName {
		t.Fatalf("this name shall be deregistered already:%v", err)
	}
}

func TestNameServerTres(t *testing.T) {
	names := []string{"uno", "dos", "tres", "cuatro", "cinco", "seis", "siete", "ocho", "nueve", "cero"}
	ns := NewNameServer()
	for _, n := range names {
		id, _ := ns.IdForName(n)
		rhs, err := ns.NameForID(id)
		if err != nil || rhs != n {
			t.Fatalf("error")
		}
	}
}
