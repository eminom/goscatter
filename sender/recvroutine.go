package sender

import (
	"bytes"
	"log"
	"net"
	"sync"
	"time"

	"github.com/eminom/go-coap"
)

// the receiving procedure
func startRecvProc(sock net.Conn, respCh chan<- *coap.Message, wg *sync.WaitGroup, whenDone func(), doneCh <-chan struct{}) {
	wg.Add(1)
	go func() {
		defer func() {
			whenDone()
			// log.Printf("leaving recv-proc")
			wg.Done()
		}()
		var ib [1024 * 1024]byte
		for {
			select {
			case <-doneCh:
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
			case <-doneCh:
				// case <-time.After(10 * time.Second):
				// log.Printf("emmm.... taking too long")
			}
		}
	}()
}
