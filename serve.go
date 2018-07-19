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
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.Ltime)

	addr, err := net.ResolveUDPAddr("udp", *fAddr)
	if err != nil {
		log.Fatalf("error resolving addr: %v", err)
	}

	ctx, doCancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	startScatter(addr, &wg, ctx.Done())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Kill, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sigCh:
	case <-ctx.Done():
	}
	log.Printf("cancelling...")
	doCancel()
	wg.Wait()

	log.Printf("bye")
}

func startScatter(addr *net.UDPAddr, wg *sync.WaitGroup, doneCh <-chan struct{}) {
	wg.Add(1)
	go runScatter(addr, wg, doneCh)
}

func runScatter(addr *net.UDPAddr, wg *sync.WaitGroup, doneCh <-chan struct{}) {
	defer func() {
		log.Printf("leaving scat-ter process")
		wg.Done()
	}()

	uSock, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}

	var bigBuff [1024 * 1024]byte
	workCh := make(chan *data.WorkItem, 1024)
	respondForbidden := func(req coap.Message) {
		workCh <- data.GenForbidden(req, addr)
	}

	sMan := data.NewServeMan(workCh)
	defer sMan.Close()

	// write-procedure. one by one.
	wg.Add(1)
	go func() {
		defer func() {
			log.Printf("leaving write-proc")
			wg.Done()
		}()
	W100:
		for {
			select {
			case <-doneCh:
				break W100
			case wi := <-workCh:
				uSock.WriteTo(wi.Buff, wi.Dest)
			}
		}

	W101:
		for {
			select {
			case <-workCh:
				// purge this channel.
			case <-time.After(2 * time.Second):
				break W101
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
				case <-doneCh:
					return
				case wi := <-bytesDisCh:
					msg, err := coap.ParseMessage(wi.Buff)
					// TODO: process more messaage type.
					if err == nil {
						switch msg.Type {
						case coap.Confirmable:
							switch msg.Code {
							case coap.POST:
								sMan.ProcessPost(msg, wi.Dest)
							case coap.GET:
								sMan.ForwardGet(msg, wi.Dest)
							case coap.PUT:
								sMan.ForwardPut(msg, wi.Dest)
							default:
								log.Printf("request type not supported: %v", msg.Type)
								respondForbidden(msg)
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
		case <-doneCh:
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

		// must make the receiving buffer duplicated.
		newWi := &data.WorkItem{
			Dest: from,
			Buff: bytes.Repeat(bigBuff[:n], 1),
		}

		select {
		case <-doneCh:
			return
		case bytesDisCh <- newWi:
		}
	}

}
