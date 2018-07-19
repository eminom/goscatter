// +build ignore

package main

import (
	"bytes"
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	co "./coaputils"
	"./comm"
	"./sche"

	"github.com/eminom/go-coap"
)

const (
	ElMaxResend = 3
)

var (
	fBind    = flag.String("bind", ":0", "local address")
	fAddr    = flag.String("addr", "localhost:16666", "host address")
	fWinSize = flag.Int("s", 32, "window size for batching")
	fUpload  = flag.String("u", "", "upload file path")
)

type MsgPack struct {
	msg      *coap.Message
	chunk    []byte
	d        func(*coap.Message) bool
	lastSent time.Time
	reTry    int
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

// return: a channel that consumes replies.
func makeSender(
	uSock *net.UDPConn,
	wg *sync.WaitGroup,
	doAbort func(),
	doneCh <-chan struct{}) (func(*coap.Message, func(*coap.Message) bool),
	func(),
	chan<- *coap.Message) {

	var lock = new(sync.Mutex)
	var cond = sync.NewCond(lock)

	doPreTrigger := func() {
		lock.Lock()
		cond.Signal()
		lock.Unlock()
	}

	frags := make(map[string]*MsgPack)
	msgCh := make(chan *coap.Message)
	// It was strange that, the buffer size is 0 perform the best !

	for b := 0; b < 1; b++ {
		wg.Add(1)
		go func(workid int) {
			defer func() {
				//log.Printf("leavin callback-relay-proc(%v)", workid)
				wg.Done()
			}()
			for {
				select {
				case <-doneCh:
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
		}(b)
	}

	wg.Add(1)
	go func() {
		defer func() {
			// log.Printf("leavin batch-proc")
			wg.Done()
		}()
		for batchOut := 0; ; batchOut++ {
			select {
			case <-doneCh:
				return
			default:
			}
			lock.Lock()
			if len(frags) == 0 {
				cond.Wait()
			}
			now := time.Now()
			// log.Printf("write iteration 1: %v", batchOut)
			for _, v := range frags {
				latest := v.lastSent.Add(5 * time.Second)
				if now.After(latest) {
					if v.reTry < ElMaxResend {
						v.reTry++
						v.lastSent = time.Now()
						// log.Printf("out: %v", co.GetMsgSig(v.msg))
						uSock.Write(v.chunk)
					} else {
						log.Printf("timeout, aborted")
						comm.SetExitCode(comm.ExitCode_ReqTimeout)
						doAbort()
						break
					}
				}
			}
			lock.Unlock()
		}
	}()

	elSend := func(req *coap.Message, dealer func(*coap.Message) bool) {
		lock.Lock()
		serialid := co.GetMsgSig(req)
		if _, ok := frags[serialid]; !ok {
			frags[serialid] = NewMsgPack(req, dealer)
			cond.Signal()
		}
		lock.Unlock()
	}

	return elSend, doPreTrigger, msgCh
}

func main() {
	defer func() {
		comm.QuitProgram()
	}()
	masterEnt()
}

func masterEnt() {
	addr, err := net.ResolveUDPAddr("udp", *fAddr)
	if nil != err {
		log.Fatalf("error resovling host address: %v", err)
	}

	esteAddr, err := net.ResolveUDPAddr("udp", *fBind)
	if nil != err {
		log.Fatalf("error resolving local address: %v", err)
	}

	sock, err := net.DialUDP("udp", esteAddr, addr)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	var wg sync.WaitGroup
	ctx, doCancel0 := context.WithCancel(context.Background())
	sender, doPreTrigger, respCh := makeSender(
		sock,
		&wg,
		func() { doCancel0() },
		ctx.Done(),
	)
	doCancel := context.CancelFunc(func() {
		doCancel0()
		doPreTrigger()
		// log.Printf("cancelling ...")
	})
	startRecvProc(sock, respCh, &wg, doCancel, ctx)

	sProc := sche.NewScheProc()
	if *fUpload != "" {
		if !comm.IsFileExists(*fUpload) {
			log.Fatalf("upload path error: %v", *fUpload)
		}
		sProc.StartWorkSeq(&wg, ctx.Done(), sche.MakeTransmitterWork(
			sProc, *fUpload, *fWinSize, sender,
			func() { doCancel() },
		))
	} else if len(flag.Args()) > 0 {
		sProc.StartWorkSeq(&wg, ctx.Done(), sche.MakeSacarWork(
			sProc, flag.Args()[0], *fWinSize, sender,
			func() { doCancel() }),
		)
	} else {
		log.Fatalf("not enough parameter")
	}
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
			// log.Printf("leaving recv-proc")
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
				// log.Printf("emmm.... taking too long")
			}
		}
	}()
}
