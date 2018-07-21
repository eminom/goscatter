package sender

import (
	"time"

	"github.com/eminom/go-coap"
)

const (
	ElMaxResend = 3
)

type Sender interface {
	SendMessage(*coap.Message, func(*coap.Message) bool)
	DoPreTrigger()
	GetMessageChan() chan<- *coap.Message
}

type elMessagePacket struct {
	msg      *coap.Message
	chunk    []byte
	d        func(*coap.Message) bool
	lastSent time.Time
	reTry    int
}

func newMsgPack(msg *coap.Message, d func(*coap.Message) bool) *elMessagePacket {
	chunk, err := msg.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return &elMessagePacket{
		msg:   msg,
		chunk: chunk,
		d:     d,
	}
}
