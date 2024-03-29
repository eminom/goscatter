//go:build ignore
// +build ignore

package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/eminom/goscatter/comm"
	"github.com/eminom/goscatter/sche"
	"github.com/eminom/goscatter/sender"
)

var (
	fBind     = flag.String("bind", ":0", "local address")
	fAddr     = flag.String("addr", "localhost:16666", "host address")
	fWinSize  = flag.Int("s", 16, "window size for batching")
	fUpload   = flag.String("u", "", "upload file path")
	fAlgo     = flag.String("a", "", "algorithm")
	fFragsize = flag.Int("f", 512, "fragment size")
	fVerbose  = flag.Bool("verbose", false, "show more logs")
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
	ctx, doCancel := context.WithCancel(context.Background())

	//
	var snder sender.Sender
	switch *fAlgo {
	default:
		fallthrough
	case "lockfree":
		log.Printf("using free-sender")
		snder = sender.NewFreeSender(
			sock,
			*fWinSize,
			&wg,
			func() { doCancel() },
			ctx.Done(),
		)
	case "signlock":
		log.Printf("using lock-sender")
		snder = sender.NewLockSender(
			sock,
			&wg,
			func() { doCancel() },
			ctx.Done(),
		)
	}

	sProc := sche.NewScheProc()
	isUploadWork := false
	if *fUpload != "" {
		isUploadWork = true
		if !comm.IsFileExists(*fUpload) {
			log.Fatalf("upload path error: %v", *fUpload)
		}
		sProc.StartWorkSeq(&wg, ctx.Done(), sche.MakeTransmitterWork(
			sProc, *fUpload, *fWinSize, *fFragsize, snder.SendMessage,
			func() { doCancel() },
			*fVerbose,
		))
	} else if len(flag.Args()) > 0 {
		sProc.StartWorkSeq(&wg, ctx.Done(), sche.MakeSacarWork(
			sProc, flag.Args()[0], *fWinSize, *fFragsize, snder.SendMessage,
			func() { doCancel() }),
		)
	} else {
		log.Fatalf("not enough parameter")
	}
	startTs := time.Now()
	sProc.KickOff(0)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Kill, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sigCh:
		log.Printf("user break")
	case <-ctx.Done():
	}
	doCancel() // can be called multiple times.
	snder.TriggerClose()
	wg.Wait()
	log.Printf("done in %v", time.Since(startTs))
	if isUploadWork {
		totSize := comm.GetFileSize(*fUpload)
		bandwidth := float64(totSize) / time.Since(startTs).Seconds()
		log.Printf("%v mps", bandwidth/(1024*1024))
	}
}
