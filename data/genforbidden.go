package data

import (
	"log"
	"net"

	"github.com/eminom/go-coap"
)

func GenForbidden(req coap.Message, addr net.Addr) *WorkItem {
	forb := coap.Message{
		Type:      coap.Acknowledgement,
		Code:      coap.Forbidden,
		MessageID: req.MessageID,
		Token:     req.Token,
		Payload:   []byte("request forbidden"),
	}
	chunk, err := forb.MarshalBinary()
	if err != nil {
		log.Fatalf("error marshal: %v", err)
	}
	return &WorkItem{addr, chunk}
}
