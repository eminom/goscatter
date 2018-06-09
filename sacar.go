// +build ignore

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	co "./coaputils"
	"./data"
	"./sche"

	"github.com/eminom/go-coap"
)

const (
	MinWinSize int32 = 4
	MaxWinSize int32 = 1024
)

var (
	fAddr    = flag.String("addr", "localhost:16666", "host address")
	fWinSize = flag.Int("winsize", 8, "window size for batching")
)

type MsgPack struct {
	msg      *coap.Message
	chunk    []byte
	d        func(*coap.Message) bool
	lastSent time.Time
}

func NewMsgPack(msg *coap.Message, d func(*coap.Message) bool) *MsgPack {
	chunk, err := msg.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return &MsgPack{
		msg:   msg,
		chunk: chunk,
		d:     d,
	}
}

func init() {
	flag.Parse()
}

func makeSender(uSock *net.UDPConn,
	wg *sync.WaitGroup, ctx context.Context) (func(*coap.Message, func(*coap.Message) bool), func(), chan<- *coap.Message) {

	var lock = new(sync.Mutex)
	var cond = sync.NewCond(lock)

	frags := make(map[string]*MsgPack)
	msgCh := make(chan *coap.Message)
	wg.Add(1)
	go func() {
		defer func() {
			log.Printf("leavin sender-proc")
			wg.Done()
		}()
		for {
			select {
			case <-ctx.Done():
				return

			case resp := <-msgCh:
				// log.Printf("resp 1")
				str := co.GetMsgSig(resp)
				lock.Lock()
				if d, ok := frags[str]; ok {
					lock.Unlock()
					if d.d(resp) {
						lock.Lock()
						// log.Printf("one old is removed")
						delete(frags, str)
						lock.Unlock()
					}
				} else {
					lock.Unlock()
				}
				// log.Printf("resp 1a")
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer func() {
			log.Printf("leavin batch-proc")
			wg.Done()
		}()
		for batchOut := 0; ; batchOut++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			lock.Lock()
			if len(frags) == 0 {
				// log.Printf("wait 1")
				cond.Wait()
				// log.Printf("wait 1 a")
			}
			now := time.Now()
			// log.Printf("write iteration 1: %v", batchOut)
			for _, v := range frags {
				latest := v.lastSent.Add(5 * time.Second)
				if now.After(latest) {
					v.lastSent = now
					// log.Printf("out: %v", co.GetMsgSig(v.msg))
					uSock.Write(v.chunk)
				} else {
					// log.Printf("not timeout yet")
				}
			}
			// log.Printf("write iteration 1a")
			lock.Unlock()
		}
	}()

	return func(req *coap.Message, dealer func(*coap.Message) bool) {
			// log.Printf("send 1")
			lock.Lock()
			serialid := co.GetMsgSig(req)
			if _, ok := frags[serialid]; !ok {
				frags[serialid] = NewMsgPack(req, dealer)
				cond.Signal() // incoming task
			}
			lock.Unlock()
			// log.Printf("send 1 a")
		}, func() {
			// log.Printf("triggers")
			lock.Lock()
			cond.Signal()
			lock.Unlock()
		}, msgCh
}

type Collector struct {
	WindowSize int

	bombRoad    int32
	segsCount   int
	filename    string
	hmac256     []byte
	sender      func(*coap.Message, func(*coap.Message) bool)
	timeStarted time.Time
}

func NewCollector(segCount int, filename string, hmac256 []byte,
	sender func(*coap.Message, func(*coap.Message) bool),
) *Collector {
	return &Collector{
		segsCount:   segCount,
		sender:      sender,
		filename:    filename,
		hmac256:     hmac256,
		timeStarted: time.Now(),
	}
}

func (c *Collector) StartCollect(whenDone func()) {
	sender := c.sender

	chunkArr := make([][]byte, c.segsCount)
	doPick := func(data []byte, id int) {
		chunkArr[id] = data
	}

	doSaveToDisk := func() {
		saveTo := filepath.Base(c.filename)
		fout, err := os.Create(saveTo)
		if err != nil {
			log.Fatalf("error create file for output: %v", err)
		}
		defer fout.Close()
		hmac := sha256.New()
		for _, dat := range chunkArr {
			hmac.Write(dat)
		}
		thisMac := hmac.Sum(nil)
		if bytes.Compare(thisMac, c.hmac256) == 0 {
			// log.Printf("hashed verified")
			var totLength int64 = 0
			for _, dat := range chunkArr {
				totLength += int64(len(dat))
				fout.Write(dat)
			}
			// log.Printf("saved.")
			elapsed := time.Now().Sub(c.timeStarted)
			bandwidth := float64(totLength) / elapsed.Seconds() / 1024
			log.Printf("%.2f kbps", bandwidth)
		} else {
			log.Printf("error hmac of SHA256")
			log.Printf("expecting: %v", hex.EncodeToString(c.hmac256))
			log.Printf("got: %v", hex.EncodeToString(thisMac))
		}
		whenDone()
	}

	bc := int32(c.WindowSize)
	if bc < MinWinSize {
		bc = MinWinSize
	} else if bc > MaxWinSize {
		bc = MaxWinSize
	}
	c.bombRoad = bc
	var doRush func(int)
	doRush = func(seqIdx int) {
		if seqIdx >= c.segsCount {
			nuevo := atomic.AddInt32(&c.bombRoad, -1)
			if nuevo == 0 {
				log.Printf("done.")
				doSaveToDisk()
			}
			return
		}
		// log.Printf("req for %v", seqIdx)
		req0 := co.NewGetReq(fmt.Sprintf("/file/%v/0", seqIdx))
		var newPiece data.Piece
		sender(req0, func(resp *coap.Message) bool {
			newPiece.Sig = bytes.Repeat(resp.Payload, 1)
			if newPiece.VerifySig() {
				// log.Printf("sig for %v is ok", seqIdx)
				req1 := co.NewGetReq(fmt.Sprintf("/file/%v/1", seqIdx))
				sender(req1, func(rsp2 *coap.Message) bool {
					newPiece.Chunk = bytes.Repeat(rsp2.Payload, 1)
					if newPiece.VerifyContent() {
						// log.Printf("chunk for %v is ok", seqIdx)
						doPick(newPiece.Chunk, seqIdx)
						doRush(seqIdx + int(bc))
						return true
					}
					return false
				})
				return true
			}
			return false
		})
	}
	for i := 0; i < int(bc); i++ {
		doRush(i)
	}
}

func main() {
	addr, err := net.ResolveUDPAddr("udp", *fAddr)
	if nil != err {
		panic(err)
	}

	sock, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	ctx, doCancel0 := context.WithCancel(context.Background())
	sender, doPreTrigger, respCh := makeSender(sock, &wg, ctx)
	doCancel := context.CancelFunc(func() {
		doCancel0()
		doPreTrigger()
		log.Printf("cancelling ...")
	})
	startRecvProc(sock, respCh, &wg, doCancel, ctx)

	sProc := sche.NewScheProc()
	var segs int
	var filename string
	workSeq := map[int]func(){
		0: func() {
			//~ Get length
			req := co.NewGetReq("/file/segs")
			sender(req, func(resp *coap.Message) bool {
				var err error
				segs, err = strconv.Atoi(string(resp.Payload))
				if err != nil {
					panic(err)
				}
				log.Printf("seg-count: %v", segs)
				sProc.KickOff(1)
				return true
			})
		},
		1: func() {
			req := co.NewGetReq("/file/name")
			sender(req, func(resp *coap.Message) bool {
				filename = string(resp.Payload)
				log.Printf("downloading file: %v", filename)
				sProc.KickOff(2)
				return true
			})
		},
		2: func() {
			req := co.NewGetReq("/file/sha256")
			sender(req, func(resp *coap.Message) bool {
				// log.Printf("sha256 for content: %v", hex.EncodeToString(resp.Payload))
				if len(resp.Payload) != sha256.Size {
					panic(fmt.Errorf("fatal error for sha256 request"))
				}
				coll := NewCollector(segs, filename, resp.Payload, sender)
				coll.WindowSize = *fWinSize
				coll.StartCollect(
					func() { doCancel() },
				)
				return true
			})
		},
	}
	sProc.StartWorkSeq(&wg, ctx.Done(), workSeq)
	sProc.KickOff(0)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Kill, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sigCh:
		log.Printf("user break")
	case <-ctx.Done():
	}
	doCancel() // can be called multiple times.
	wg.Wait()
	log.Printf("done")
}

// the receiving procedure
func startRecvProc(sock net.Conn, respCh chan<- *coap.Message, wg *sync.WaitGroup, doCancel context.CancelFunc, ctx context.Context) {
	wg.Add(1)
	go func() {
		defer func() {
			doCancel()
			log.Printf("leaving recv-proc")
			wg.Done()
		}()
		var ib [1024 * 1024]byte
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			sock.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, err := sock.Read(ib[:])
			if err != nil {
				if nErr, ok := err.(net.Error); ok && nErr.Timeout() {
					continue
				}
				log.Fatalf("error read: %v", err)
			}
			msg, err := coap.ParseMessage(bytes.Repeat(ib[:n], 1))
			if err != nil {
				log.Fatalf("error recv: %v", err)
			}
			// log.Printf("new msg in: %v", data.GetMsgSig(&msg))
			select {
			case respCh <- &msg: // OK. GO ON.
			case <-time.After(10 * time.Second):
				log.Printf("emmm.... taking too long")
			}
		}
	}()
}
