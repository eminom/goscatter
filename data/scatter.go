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
	"sync/atomic"
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

type IScatter interface {
	IClean
	IRefCount
	RipUp(instrs []string, req *coap.Message, from net.Addr)
	DoStop()
	GetID() int
}

type Scatter struct {
	*Fragger

	oCh      chan<- *WorkItem
	markChWR chan<- string

	doStop func()

	isDebug  bool
	scaID    int
	refCount int32

	heartBeatCh chan<- struct{}
}

// when-finalize: can be called only once.
func NewScatter(inpath string, shortid int, ch chan<- *WorkItem, whenFinalize func()) IScatter {
	markCh := make(chan string, 16)
	runCtx, doStopAhora := context.WithCancel(context.Background())
	resCh := make(chan int, 2)

	var wg sync.WaitGroup

	// The exit point>>
	var stopOnce sync.Once
	stopFunc := func() {
		stopOnce.Do(func() {
			log.Printf("%v is quitting.", inpath)
			whenFinalize()
			doStopAhora()
		})
	}

	timeoutDuration := 15 * time.Second
	if IsSpecialDirName(inpath) {
		var e1 error
		timeoutDuration, e1 = time.ParseDuration(fmt.Sprintf("%vh", 365*100*24))
		if e1 != nil {
			panic(e1)
		}
	}

	//
	hbChan := make(chan struct{}, 1)
	rv := &Scatter{
		isDebug:  fDebugDups, //
		Fragger:  NewFragger(inpath),
		oCh:      ch,
		markChWR: markCh,
		scaID:    shortid,
		refCount: 1,
		doStop: func() {
			stopFunc()
			dups := <-resCh
			alls := <-resCh
			if fDebugDups {
				log.Printf("info for %v", inpath)
				log.Printf("dups count: %v", dups)
				log.Printf("%v in all.", alls)
			}
			log.Printf("%v(%v) halt *****", inpath, shortid)
			wg.Wait()
		},
		heartBeatCh: hbChan,
	}

	// Now go
	wg.Add(1)
	go func() {
		dupsCount, totsCount := 0, 0
		defer func() {
			stopFunc()
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
			case <-time.After(timeoutDuration):
				if rv.Release() {
					return
				}
				//TODO: make this 15 parameterized.
			case <-hbChan:
			}
		}
	}()

	if IsSpecialDirName(inpath) {
		return &ScatterLive{
			Scatter: rv,
		}
	}
	return rv
}

func (s *Scatter) GetID() int {
	return s.scaID
}

// true for ready to release.
func (s *Scatter) Release() bool {
	v := atomic.AddInt32(&s.refCount, -1)
	return 0 == v
}

func (s *Scatter) AddRef() {
	atomic.AddInt32(&s.refCount, 1)
}

// Technically, it can be called for only once.
func (s *Scatter) DoStop() {
	if s.Release() {
		s.doStop()
	}
}

func (s *Scatter) DoClean() {
	log.Printf("scatter<%v> cleaning up ...", s.originalName)
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

type ScatterLive struct {
	*Scatter
}

func (s *ScatterLive) DoStop() {
	//
	if s.Scatter.isDebug {
		log.Printf("no i do not stop")
	}
}
