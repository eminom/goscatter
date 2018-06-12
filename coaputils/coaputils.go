package coaputils

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/eminom/go-coap"
)

var (
	msgIDer = NewMsgIDGen()
)

func GetMsgSig(msg *coap.Message) string {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], msg.MessageID)
	return hex.EncodeToString(msg.Token) + "-" + hex.EncodeToString(b[:])
}

func newGetReq(reqPath string) *coap.Message {
	rv := &coap.Message{
		Type:      coap.Confirmable,
		Code:      coap.GET,
		MessageID: genMessageID(),
		Token:     genSerial(8),
	}
	rv.SetPathString(reqPath)
	return rv
}

func NewGetReqf(format string, a ...interface{}) *coap.Message {
	return newGetReq(fmt.Sprintf(format, a...))
}

func NewPutReqf(format string, a ...interface{}) *coap.Message {
	rv := newGetReq(fmt.Sprintf(format, a...))
	rv.Code = coap.PUT
	return rv
}

func NewPostReqf(format string, a ...interface{}) *coap.Message {
	rv := newGetReq(fmt.Sprintf(format, a...))
	rv.Code = coap.POST
	return rv
}

func genSerial(n int) []byte {
	var buf [1024]byte
	rand.Read(buf[:n])
	return buf[:n]
}

func genMessageID() uint16 {
	return msgIDer.NextID()
}
