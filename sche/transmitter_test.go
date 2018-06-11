package sche

import (
	"testing"
)

func TestSimple(t *testing.T) {
	inputs := []string{"1a", "03", "ff", "Fe", "e0"}
	for _, in := range inputs {
		if !isHexString.MatchString(in) {
			t.Fatalf("error")
		}
	}
}
