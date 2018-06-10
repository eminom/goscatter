package data

import (
	"net"

	"github.com/eminom/go-coap"
)

func pushToOutch(resp *coap.Message, addr net.Addr, oCh chan<- *WorkItem) {
	ob, err := resp.MarshalBinary()
	if err != nil {
		panic(err)
	}
	oCh <- &WorkItem{addr, ob}
}
