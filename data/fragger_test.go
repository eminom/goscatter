package data

import (
	"testing"
)

func TestFraggerLoad(t *testing.T) {
	NewFragger("fragger.go")
	t.Logf("done")
}
