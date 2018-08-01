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

type freeSender struct {
	elSend func(msg *coap.Message, callback func(*coap.Message) bool)
	msgCh  chan<- *coap.Message
}

type reqItem struct {
	req *coap.Message
	d   func(*coap.Message) bool
}

func biggerOne(a, b int) int {
	if a < b {
		return b
	}
	return a
}

func NewFreeSender(uSock *net.UDPConn, wsize int, wg *sync.WaitGroup, doAbort func(), doneCh <-chan struct{}) Sender {
	rv := newFreeSender(uSock, wsize, wg, doAbort, doneCh)
	startRecvProc(uSock, rv.msgCh, wg, doAbort, doneCh)
	return rv
}

func newFreeSender(uSock *net.UDPConn, wsize int, wg *sync.WaitGroup, doAbort func(), doneCh <-chan struct{}) *freeSender {

	rv := &freeSender{}

	msgCh := make(chan *coap.Message, wsize)
	inMessageCh := make(chan *reqItem, biggerOne(1024, wsize)) //: bigger, the better

	for b := 0; b < 1; b++ {
		wg.Add(1)
		go func(workid int) {
			defer func() {
				// log.Printf("leavin callback-relay-proc(%v)", workid)
				// it seems unnecessar to purge message-channel
				// cause buffer size is assured of being no less
				// than (concurrent) senders
			W100:
				for {
					select {
					case <-inMessageCh:
					default:
						break W100
					}
				}
				wg.Done()
			}()
			frags := make(map[string]*elMessagePacket)
			ticker := time.NewTicker(5 * time.Second)
			for {
				select {
				case <-doneCh:
					ticker.Stop()
					return

				case resp := <-msgCh:
					if resp.Type != coap.Acknowledgement {
						log.Printf("unexpected type")
						return
					}
					str := co.GetMsgSig(resp)
					if d, ok := frags[str]; ok {
						if d.d(resp) {
							delete(frags, str)
						} else {
							log.Printf("rejected: %v", resp)
						}
					}

				case newReq := <-inMessageCh:
					serialid := co.GetMsgSig(newReq.req)
					var nq *elMessagePacket
					if _, ok := frags[serialid]; !ok {
						nq = newMsgPack(newReq.req, newReq.d)
						frags[serialid] = nq
						nq.hacerSend(uSock)
					}
				case <-ticker.C:
					now := time.Now()
					// log.Printf("write iteration 1: %v", batchOut)
					for _, v := range frags {
						latest := v.lastSent.Add(5 * time.Second)
						if now.After(latest) {
							if v.reTry < ElMaxResend {
								// log.Printf("out: %v", co.GetMsgSig(v.msg))
								v.hacerSend(uSock)
							} else {
								log.Printf("timeout, aborted")
								comm.SetExitCode(comm.ExitCode_ReqTimeout)
								doAbort()
								break
							}
						}
					}
				}
			}
		}(b)
	}

	rv.msgCh = msgCh

	rv.elSend = func(req *coap.Message, dealer func(*coap.Message) bool) {
		inMessageCh <- &reqItem{
			req: req,
			d:   dealer,
		}
	}
	return rv
}

func (ls *freeSender) TriggerClose() {}

//
func (ls *freeSender) SendMessage(msg *coap.Message, callback func(*coap.Message) bool) {
	ls.elSend(msg, callback)
}
