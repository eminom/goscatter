// +build ignore

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
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

	"./data"

	"github.com/eminom/go-coap"
)

var (
	fAddr = flag.String("addr", "localhost:16666", "host address")
)

var (
	msgIDer = data.NewMsgIDGen()
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

func getMsgSig(msg *coap.Message) string {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], msg.MessageID)
	return hex.EncodeToString(msg.Token) + "-" + hex.EncodeToString(b[:])
}

func makeSender(uSock *net.UDPConn,
	wg *sync.WaitGroup, ctx context.Context) (func(*coap.Message, func(*coap.Message) bool), func(), chan<- *coap.Message) {

	var lock = new(sync.Mutex)
	var cond = sync.NewCond(lock)

	frags := make(map[string]*MsgPack)
	// obsoletes := make(map[string]bool)
	msgCh := make(chan *coap.Message)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return

			case resp := <-msgCh:
				// log.Printf("resp 1")
				str := getMsgSig(resp)
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
		defer wg.Done()
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
					// log.Printf("out: %v", getMsgSig(v.msg))
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
			serialid := getMsgSig(req)
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

func newGetReq(reqPath string) *coap.Message {
	rv := &coap.Message{
		Type:      coap.Confirmable,
		Code:      coap.GET,
		MessageID: genMessageID(),
		Token:     genSerial(8),
	}
	rv.SetPathString(reqPath)
	return rv
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

	const bc = 4
	c.bombRoad = 4
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
		req0 := newGetReq(fmt.Sprintf("/file/%v/0", seqIdx))
		var newPiece data.Piece
		sender(req0, func(resp *coap.Message) bool {
			newPiece.Sig = bytes.Repeat(resp.Payload, 1)
			if newPiece.VerifySig() {
				// log.Printf("sig for %v is ok", seqIdx)
				req1 := newGetReq(fmt.Sprintf("/file/%v/1", seqIdx))
				sender(req1, func(rsp2 *coap.Message) bool {
					newPiece.Chunk = bytes.Repeat(rsp2.Payload, 1)
					if newPiece.VerifyContent() {
						// log.Printf("chunk for %v is ok", seqIdx)
						doPick(newPiece.Chunk, seqIdx)
						doRush(seqIdx + bc)
						return true
					}
					return false
				})
				return true
			}
			return false
		})
	}
	for i := 0; i < bc; i++ {
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
	})
	startRecvProc(sock, respCh, &wg, doCancel, ctx)

	wg.Add(1)
	go func() {
		defer func() {
			wg.Done()
			// log.Printf("leavin stepper")
		}()
		var segs int
		var filename string
		procCh := make(chan int, 1)
		procCh <- 0
		// log.Printf("started")
		var step int
		for {
			select {
			case step = <-procCh:
			case <-ctx.Done():
				return
			}
			// log.Printf("step: %v", step)
			switch step {
			case 0:
				//~ Get length
				req := newGetReq("/file/segs")
				sender(req, func(resp *coap.Message) bool {
					var err error
					segs, err = strconv.Atoi(string(resp.Payload))
					if err != nil {
						panic(err)
					}
					log.Printf("seg-count: %v", segs)
					procCh <- 1
					return true
				})
			case 1:
				req := newGetReq("/file/name")
				sender(req, func(resp *coap.Message) bool {
					filename = string(resp.Payload)
					log.Printf("downloading file: %v", filename)
					procCh <- 2
					return true
				})
			case 2:
				req := newGetReq("/file/sha256")
				sender(req, func(resp *coap.Message) bool {
					// log.Printf("sha256 for content: %v", hex.EncodeToString(resp.Payload))
					if len(resp.Payload) != sha256.Size {
						panic(fmt.Errorf("fatal error for sha256 request"))
					}
					NewCollector(segs, filename, resp.Payload, sender).StartCollect(
						func() { doCancel() },
					)
					return true
				})
			default:
				log.Fatalf("error: %v", step)
			}
		}
	}()

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
			// log.Printf("leaving recv-proc")
			wg.Done()
			doCancel()
			// log.Printf("leaving recv-proc 1a")
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
			// log.Printf("new msg in: %v", getMsgSig(&msg))
			respCh <- &msg // OK. GO ON.
		}
	}()
}

func genSerial(n int) []byte {
	var buf [1024]byte
	rand.Read(buf[:n])
	return buf[:n]
}

func genMessageID() uint16 {
	return msgIDer.NextID()
}
