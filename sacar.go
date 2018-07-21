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

	"./comm"
	"./sche"
	"./sender"

	"github.com/eminom/go-coap"
)

var (
	fBind    = flag.String("bind", ":0", "local address")
	fAddr    = flag.String("addr", "localhost:16666", "host address")
	fWinSize = flag.Int("s", 16, "window size for batching")
	fUpload  = flag.String("u", "", "upload file path")
)

func init() {
	flag.Parse()
}

func main() {
	defer func() {
		comm.QuitProgram()
	}()
	log.SetFlags(log.Lmicroseconds)
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
	// sender := sender.NewLockSender(
	// 	sock,
	// 	&wg,
	// 	func() { doCancel0() },
	// 	ctx.Done(),
	// )

	sender := sender.NewFreeSender(
		sock,
		*fWinSize,
		&wg,
		func() { doCancel0() },
		ctx.Done(),
	)

	doCancel := context.CancelFunc(func() {
		doCancel0()
		sender.DoPreTrigger()
		// log.Printf("cancelling ...")
	})
	startRecvProc(sock, sender.GetMessageChan(), &wg, doCancel, ctx)

	sProc := sche.NewScheProc()
	if *fUpload != "" {
		if !comm.IsFileExists(*fUpload) {
			log.Fatalf("upload path error: %v", *fUpload)
		}
		sProc.StartWorkSeq(&wg, ctx.Done(), sche.MakeTransmitterWork(
			sProc, *fUpload, *fWinSize, sender.SendMessage,
			func() { doCancel() },
		))
	} else if len(flag.Args()) > 0 {
		sProc.StartWorkSeq(&wg, ctx.Done(), sche.MakeSacarWork(
			sProc, flag.Args()[0], *fWinSize, sender.SendMessage,
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
			case <-ctx.Done():
				// case <-time.After(10 * time.Second):
				// log.Printf("emmm.... taking too long")
			}
		}
	}()
}
