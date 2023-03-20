package sender

import (
	"log"
	"net"
	"sync"
	"time"

	co "github.com/eminom/goscatter/coaputils"
	"github.com/eminom/goscatter/comm"

	"github.com/eminom/go-coap"
)

type lockSender struct {
	elSend       func(msg *coap.Message, callback func(*coap.Message) bool)
	doPreTrigger func()
	msgCh        chan<- *coap.Message
}

func NewLockSender(uSock *net.UDPConn, wg *sync.WaitGroup, doAbort func(), doneCh <-chan struct{}) Sender {
	rv := newLockSender(uSock, wg, doAbort, doneCh)
	startRecvProc(uSock, rv.msgCh, wg, doAbort, doneCh)
	return rv
}

func newLockSender(uSock *net.UDPConn, wg *sync.WaitGroup, doAbort func(), doneCh <-chan struct{}) *lockSender {

	rv := &lockSender{}

	var lock = new(sync.Mutex)
	var cond = sync.NewCond(lock)

	rv.doPreTrigger = cond.Signal

	frags := make(map[string]*elMessagePacket)
	msgCh := make(chan *coap.Message, 16)
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

func (ls *lockSender) TriggerClose() {
	ls.doPreTrigger()
}

func (ls *lockSender) SendMessage(msg *coap.Message, callback func(*coap.Message) bool) {
	ls.elSend(msg, callback)
}
