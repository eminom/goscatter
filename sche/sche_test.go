package sche

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"
)

func TestSche(t *testing.T) {
	ctx, doCancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	s := NewScheProc()
	s.StartWorkSeq(&wg, ctx.Done(), map[int]func(){
		2: func() {
			time.AfterFunc(3*time.Millisecond, func() {
				log.Printf("A1")
				s.KickOff(3)
			})
		},
		3: func() {
			time.AfterFunc(2*time.Millisecond, func() {
				log.Printf("A2")
				s.KickOff(5)
			})
		},
		5: func() {
			doCancel()
		},
	})
	s.KickOff(2)
	wg.Wait()
	t.Logf("work-seq-test done")
}
