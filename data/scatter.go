package data

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"

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

	runCtx   context.Context
	doStop   context.CancelFunc
	resOutCh <-chan int
}

func NewScatter(inpath string, ch chan<- *WorkItem) *Scatter {
	markCh := make(chan string, 16)
	rv := &Scatter{
		Fragger:  NewFragger(inpath),
		oCh:      ch,
		markChWR: markCh,
	}

	rv.runCtx, rv.doStop = context.WithCancel(context.Background())

	resCh := make(chan int)
	rv.resOutCh = resCh

	go func() {
		dupsCount, totsCount := 0, 0
		defer func() {
			resCh <- dupsCount
			resCh <- totsCount
		}()

		statMap := make(map[string]int)
		for {
			select {
			case <-rv.runCtx.Done():
				return
			case newMark := <-markCh:
				totsCount++
				statMap[newMark] += 1
				if statMap[newMark] > 1 {
					log.Printf("duplicate for %v", newMark)
					dupsCount++
				}
			}
		}
	}()

	return rv
}

func (s *Scatter) DoStop() {
	s.doStop()
	dups := <-s.resOutCh
	alls := <-s.resOutCh
	log.Printf("dups count: %v", dups)
	log.Printf("%v in all.", alls)
}

func (s *Scatter) Dispatch(req coap.Message, from net.Addr) {
	// log.Printf("dispatching")
	paths := req.Path()

	var resp *coap.Message
	(func() {
		if len(paths) >= 2 {
			if paths[0] == "file" {
				if IsDigit.MatchString(paths[1]) {
					if idx, err := strconv.Atoi(paths[1]); nil == err && idx >= 0 && idx < s.segCount {
						if len(paths) >= 3 {
							if sel, err := strconv.Atoi(paths[2]); nil == err && sel >= 0 && sel < 2 {
								// 0 for sig
								switch sel {
								case 0:
									resp = &coap.Message{
										Code:    coap.Content,
										Payload: s.chunks[idx].Sig,
									}
								case 1:
									resp = &coap.Message{
										Code:    coap.Content,
										Payload: s.chunks[idx].Chunk,
									}
								default:
									log.Printf("invalid sub[2]")
								}
							}
						}
					}
				} else {
					switch paths[1] {
					case "segs":
						resp = &coap.Message{
							Code:    coap.Content,
							Payload: []byte(fmt.Sprintf("%v", s.segCount)),
						}
					case "name":
						resp = &coap.Message{
							Code:    coap.Content,
							Payload: []byte(fmt.Sprintf("%v", s.originalName)),
						}
					case "sha256":
						// log.Printf("responed with hash: %v", hex.EncodeToString(s.hashSha256))
						resp = &coap.Message{
							Code:    coap.Content,
							Payload: s.hashSha256,
						}
					default:
						log.Printf("unknown <%v>", paths[1])
					}
				}
			} else {
				log.Printf("unknown first-level <%v>", paths[0])
			}
		} else {
			log.Printf("unknown uri-path: <%v>", strings.Join(paths, "/"))
		}
	})()

	if nil == resp {
		resp = &coap.Message{
			Code:    coap.BadRequest,
			Payload: []byte("bad request"),
		}
	} else {
		// mark or not.
		if fDebugDups {
			s.markChWR <- fmt.Sprintf("%x", req.MessageID) + "-" + hex.EncodeToString(req.Token)
		}
	}

	// fin
	resp.Type = coap.Acknowledgement
	resp.MessageID = req.MessageID
	resp.Token = req.Token
	pushToOutch(resp, from, s.oCh)
}
