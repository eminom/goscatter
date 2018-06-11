package sche

import (
	"log"
	"sync"
)

type Sche interface {
	KickOff(int)
	StartWorkSeq(*sync.WaitGroup, <-chan struct{}, map[int]func())
}

type scheProc struct {
	procCh chan int
}

func NewScheProc() *scheProc {
	rv := &scheProc{
		procCh: make(chan int),
	}
	return rv
}

func (s *scheProc) KickOff(v int) {
	s.procCh <- v
}

func (s *scheProc) StartWorkSeq(wg *sync.WaitGroup, doneCh <-chan struct{}, workMan map[int]func()) {
	wg.Add(1)
	go func() {
		defer func() {
			log.Printf("leavin seq-work")
			wg.Done()
		}()
		var workI int
		for {
			select {
			case workI = <-s.procCh:
			case <-doneCh:
				return
			}
			if work, ok := workMan[workI]; ok {
				work()
			} else {
				log.Printf("error work id: %v", workI)
			}
		}
	}()
}
