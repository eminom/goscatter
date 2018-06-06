package data

import (
	"fmt"
	"testing"
)

func genMaker() (func(interface{}), *string) {
	var out string
	return func(v interface{}) {
		out += fmt.Sprintf("%v,", v)
	}, &out
}

func TestWorknodelist(t *testing.T) {

	l := NewWorkNodeList()
	for _, v := range []int{2, 3, 5, 7} {
		l.Append(v)
	}

	t.Log("***************")
	uno, res1 := genMaker()
	l.Walk(uno)
	t.Logf("%v", *res1)
	if *res1 != "2,3,5,7," {
		t.Fatalf("error")
	}

	l.Remove(4)
	l.Remove(3)
	l.Remove(7)

	t.Log("****************")
	dos, res2 := genMaker()
	l.Walk(dos)
	t.Logf("%v", *res2)
	if *res2 != "2,5," {
		t.Fatalf("error")
	}

	t.Log("**************")
	l.Shift()
	l.Shift()
	l.Append(8)
	tres, res3 := genMaker()
	l.Walk(tres)
	t.Logf("%v", *res3)
	if *res3 != "8," {
		t.Fatalf("error")
	}

}
