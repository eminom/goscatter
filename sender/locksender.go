package sender

import (
	"log"
	"net"
	"sync"
	"time"

	co "../coaputils"
	"../comm"

	"github.com/eminom/go-coap"
)

type lockSender struct {
	elSend       func(msg *coap.Message, callback func(*coap.Message) bool)
	doPreTrigger func()
	msgCh        chan<- *coap.Message
}

func NewLockSender(uSock *net.UDPConn, wg *sync.WaitGroup, doAbort func(), doneCh <-chan struct{}) Sender {

	rv := &lockSender{}

	var lock = new(sync.Mutex)
	var cond = sync.NewCond(lock)

	rv.doPreTrigger = func() {
		lock.Lock()
		cond.Signal()
		lock.Unlock()
	}

	frags := make(map[string]*elMessagePacket)
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
						} else {
							log.Printf("rejected: %v", resp)
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

	rv.msgCh = msgCh

	rv.elSend = func(req *coap.Message, dealer func(*coap.Message) bool) {
		lock.Lock()
		serialid := co.GetMsgSig(req)
		if _, ok := frags[serialid]; !ok {
			frags[serialid] = newMsgPack(req, dealer)
			cond.Signal()
		}
		lock.Unlock()
	}

	return rv
}

func (ls *lockSender) DoPreTrigger() {
	ls.doPreTrigger()
}

//
func (ls *lockSender) SendMessage(msg *coap.Message, callback func(*coap.Message) bool) {
	ls.elSend(msg, callback)
}

// write-only channel
func (ls *lockSender) GetMessageChan() chan<- *coap.Message {
	return ls.msgCh
}
