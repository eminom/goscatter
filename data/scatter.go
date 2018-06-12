package data

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/eminom/go-coap"
)

var (
	IsDigit = regexp.MustCompile(`^\d+$`)
)

var (
	fDebugDups = false
)

type WorkItem struct {
	Dest net.Addr
	Buff []byte
}

type Scatter struct {
	*Fragger

	oCh      chan<- *WorkItem
	markChWR chan<- string

	doStop func()

	isDebug bool

	heartBeatCh chan<- struct{}
}

// when-finalize: can be called only once.
func NewScatter(inpath string, shortid int, ch chan<- *WorkItem, whenFinalize func()) *Scatter {
	markCh := make(chan string, 16)
	runCtx, doStopAhora := context.WithCancel(context.Background())
	resCh := make(chan int, 2)

	var wg sync.WaitGroup

	// The exit point>>
	var stopOnce sync.Once
	stopFunc := func() {
		stopOnce.Do(func() {
			doStopAhora()
			whenFinalize()
		})
	}

	hbChan := make(chan struct{}, 1)
	// Now go
	wg.Add(1)
	go func() {
		dupsCount, totsCount := 0, 0
		defer func() {
			resCh <- dupsCount
			resCh <- totsCount
			wg.Done()
		}()
		statMap := make(map[string]int)
		for {
			select {
			case <-runCtx.Done():
				return

				// This is for debugging purpose:
			case newMark := <-markCh:
				totsCount++
				statMap[newMark] += 1
				if statMap[newMark] > 1 {
					log.Printf("duplicate for %v", newMark)
					dupsCount++
				}
			case <-time.After(15 * time.Second):
				stopFunc()
				return
				//TODO: make this 15 parameterized.
			case <-hbChan:
			}
		}
	}()

	return &Scatter{
		isDebug:  fDebugDups, //
		Fragger:  NewFragger(inpath),
		oCh:      ch,
		markChWR: markCh,
		doStop: func() {
			stopFunc()
			dups := <-resCh
			alls := <-resCh
			log.Printf("dups count: %v", dups)
			log.Printf("%v in all.", alls)
			wg.Wait()
		},
		heartBeatCh: hbChan,
	}
}

func (s *Scatter) DoStop() {
	s.doStop()
}

func (s *Scatter) RipUp(instrs []string, req *coap.Message, from net.Addr) {
	var resp = &coap.Message{
		Code:      coap.BadRequest,
		MessageID: req.MessageID,
		Token:     req.Token,
		Payload:   []byte("bad request"),
	}
	var idxstr, sel string
	if len(instrs) >= 1 {
		idxstr = instrs[0]
	}
	if len(instrs) >= 2 {
		sel = instrs[1]
	}
	if IsDigit.MatchString(idxstr) {
		idx, err := strconv.Atoi(idxstr)
		selIdx, err1 := strconv.Atoi(sel)
		if nil == err && nil == err1 && idx >= 0 && idx < s.segCount {
			// 0 for sig
			switch selIdx {
			case 0:
				resp.Code = coap.Content
				resp.Payload = s.chunks[idx].Sig
			case 1:
				resp.Code = coap.Content
				resp.Payload = s.chunks[idx].Chunk
			default:
				log.Printf("invalid sel-value [%v]", sel)
			}
		} else {
			log.Printf("format error for get: <%v/%v>", idxstr, sel)
		}
	} else {
		switch idxstr {
		case "segs":
			resp.Code = coap.Content
			resp.Payload = []byte(fmt.Sprintf("%v", s.segCount))
		case "name":
			resp.Code = coap.Content
			resp.Payload = []byte(fmt.Sprintf("%v", s.originalName))
		case "sha256":
			resp.Code = coap.Content
			resp.Payload = s.hashSha256
		default:
			log.Printf("unknown <%v>", idxstr)
		}
	}

	// mark or not.
	if s.isDebug {
		s.markChWR <- fmt.Sprintf("%x", req.MessageID) + "-" + hex.EncodeToString(req.Token)
	}

	// heart beat.
	s.heartBeatCh <- struct{}{}

	// fin
	pushToOutch(resp, from, s.oCh)
}
