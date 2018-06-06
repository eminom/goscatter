// +build ignore

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
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
	return hex.EncodeToString(msg.Token) + hex.EncodeToString(b[:])
}

func makeSender(uSock *net.UDPConn,
	wg *sync.WaitGroup, ctx context.Context) (func(*coap.Message, func(*coap.Message) bool), chan<- *coap.Message) {

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
	}, msgCh
}

type Collector struct {
	bombRoad  int32
	segsCount int
	sender    func(*coap.Message, func(*coap.Message) bool)
}

func NewCollector(segCount int, sender func(*coap.Message, func(*coap.Message) bool)) *Collector {
	return &Collector{
		segsCount: segCount,
		sender:    sender,
	}
}

func newReq() *coap.Message {
	return &coap.Message{
		Type:      coap.Confirmable,
		Code:      coap.GET,
		MessageID: genMessageID(),
		Token:     genSerial(8),
	}
}

func (c *Collector) StartCollect() {
	sender := c.sender

	chunkArr := make([][]byte, c.segsCount)
	doPick := func(data []byte, id int) {
		chunkArr[id] = data
	}

	doSaveToDisk := func() {
		fout, err := os.Create("save.bin")
		if err != nil {
			log.Fatalf("error create file for output: %v", err)
		}
		defer fout.Close()
		for _, dat := range chunkArr {
			fout.Write(dat)
		}
		log.Printf("saved")
	}

	const bc = 4
	c.bombRoad = 4
	var doRush func(int)
	doRush = func(seqIdx int) {
		if seqIdx >= c.segsCount {
			nuevo := atomic.AddInt32(&c.bombRoad, -1)
			if nuevo == 0 {
				log.Printf("done!")
				doSaveToDisk()
			}
			return
		}
		log.Printf("req for %v", seqIdx)
		req0 := newReq()
		req0.SetPathString(fmt.Sprintf("/file/%v/0", seqIdx))
		var newPiece data.Piece
		sender(req0, func(resp *coap.Message) bool {
			newPiece.Sig = bytes.Repeat(resp.Payload, 1)
			if newPiece.VerifySig() {
				// log.Printf("sig for %v is ok", seqIdx)
				req1 := newReq()
				req1.SetPathString(fmt.Sprintf("/file/%v/1", seqIdx))
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
	ctx, doCancel := context.WithCancel(context.Background())
	sender, respCh := makeSender(sock, &wg, ctx)

	//~ Get length
	req := coap.Message{
		Type:      coap.Confirmable,
		Code:      coap.GET,
		MessageID: genMessageID(),
		Token:     genSerial(8),
	}
	req.SetPathString("/file/segs")

	sender(&req, func(resp *coap.Message) bool {
		segs, err := strconv.Atoi(string(resp.Payload))
		if err != nil {
			panic(err)
		}
		log.Printf("seg-count: %v", segs)
		NewCollector(segs, sender).StartCollect()
		return true
	})

	// the receiving procedure
	wg.Add(1)
	go func() {
		defer func() {
			wg.Done()
			doCancel()
		}()
		var ib [1024 * 1024]byte
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			sock.SetReadDeadline(time.Now().Add(5 * time.Second))
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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Kill, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sigCh:
	case <-ctx.Done():
	}
	doCancel() // can be called multiple times.
	wg.Wait()
	log.Printf("done")
}

func genSerial(n int) []byte {
	var buf [1024]byte
	rand.Read(buf[:n])
	return buf[:n]
}

func genMessageID() uint16 {
	return msgIDer.NextID()
}
