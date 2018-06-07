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

	"./data"

	"github.com/eminom/go-coap"
)

func main() {
	var fAddr = flag.String("addr", ":16666", "address for listening")
	var fContent = flag.String("target", "tmp/bin.7z", "zip")
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.Ltime)

	addr, err := net.ResolveUDPAddr("udp", *fAddr)
	if err != nil {
		panic(err)
	}

	startScatter(addr, *fContent)

	log.Printf("bye")
}

func startScatter(addr *net.UDPAddr, inputpath string) {

	ctx, doCancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go runScatter(addr, inputpath, &wg, ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Kill, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	log.Printf("cancelling...")
	doCancel()
	wg.Wait()
}

func runScatter(addr *net.UDPAddr, inpath string, wg *sync.WaitGroup, ctx context.Context) {
	defer func() {
		log.Printf("leaving scatter process")
		wg.Done()
	}()

	uSock, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}

	var bigBuff [1024 * 1024]byte
	workCh := make(chan *data.WorkItem, 1024)

	scatter := data.NewScatter(inpath, workCh)
	log.Printf("scatter ready")
	defer scatter.DoStop()

	// write-procedure. one by one.
	wg.Add(1)
	go func() {
		defer func() {
			log.Printf("leaving write-proc")
			wg.Done()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case wi := <-workCh:
				uSock.WriteTo(wi.Buff, wi.Dest)
			}
		}
	}()

	const ParserCount = 4
	bytesDisCh := make(chan *data.WorkItem, 1024)
	for i := 0; i < ParserCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case wi := <-bytesDisCh:
					msg, err := coap.ParseMessage(wi.Buff)
					// TODO: process more messaage type.
					if err == nil {
						switch msg.Type {
						case coap.Confirmable:
							if msg.Code == coap.GET {
								scatter.Dispatch(msg, wi.Dest)
							} else {
								log.Printf("request type not supported: %v", msg.Type)
								scatter.GenForbidden(msg, wi.Dest)
							}
						default:
							log.Printf("ignore msg: %v", msg.Type)
						}
					} else {
						log.Printf("not CoAP")
					}
				}
			}
		}()
	}

	// reader procedure
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		uSock.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, from, err := uSock.ReadFrom(bigBuff[:])
		if err != nil {
			if nErr, ok := err.(net.Error); !ok || !nErr.Temporary() {
				log.Printf("network error: %T: %v", err, err)
				return
			}
			continue
		}

		// log.Printf("new req")
		// must make the receiving buffer duplicated.
		newWi := &data.WorkItem{from, bytes.Repeat(bigBuff[:n], 1)}
	S100:
		for {
			select {
			case bytesDisCh <- newWi:
				break S100
			default:
				log.Printf("buff channel cramped **")
				time.Sleep(1 * time.Second)
			}
		}
	}

}
