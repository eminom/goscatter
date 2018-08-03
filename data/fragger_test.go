package data

import (
	"testing"
)

func TestFraggerLoad(t *testing.T) {
	NewFragger(3, "fragger.go")
	t.Logf("done")
}
