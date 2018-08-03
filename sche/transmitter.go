package sche

import (
	"log"
	"path/filepath"
	"regexp"
	"sync/atomic"

	co "../coaputils"
	"../comm"
	"../data"

	"github.com/eminom/go-coap"
)

var (
	isHexString = regexp.MustCompile(`^[\dA-Fa-f]+$`)
)

//
const (
	DefaultTransmitterSegmentSize = 512
)

func MakeTransmitterWork(proc Sche, inpath string, winSize int,
	sender func(*coap.Message, func(*coap.Message) bool),
	doFinish func()) map[int]func() {

	fragger := data.NewFragger(DefaultTransmitterSegmentSize, inpath)
	pieces := fragger.GetPieces()
	lPiece := len(pieces)
	trait := data.MakeComposerTrait(fragger.GetHash(), lPiece)

	bc := winSize
	if bc > int(MaxWinSize) {
		bc = int(MaxWinSize)
	} else if bc < int(MinWinSize) {
		bc = int(MinWinSize)
	}

	var rLeft = int32(bc)
	var shortid string // something like 0xE6

	var doUpload func(int)
	doUpload = func(idx int) {
		if idx >= lPiece {
			nuevo := atomic.AddInt32(&rLeft, -1)
			if 0 == nuevo {
				//TODO
				log.Printf("all uploaded")
				proc.KickOff(2)
			}
			return
		}
		if idx%1000 == 0 {
			log.Printf("upload for %v", idx)
		}
		req := co.NewPutReqf("/f/%v/%v/0", shortid, idx)
		req.Payload = pieces[idx].Sig
		sender(req, func(resp *coap.Message) bool {
			if coap.Changed == resp.Code {
				req1 := co.NewPutReqf("/f/%v/%v/1", shortid, idx)
				req1.Payload = pieces[idx].Chunk
				sender(req1, func(resp *coap.Message) bool {
					if coap.Changed == resp.Code {
						doUpload(idx + bc)
						return true
					}
					return false
				})
				return true
			}
			log.Printf("rejected for %v", req.Path())
			return false
		})
	}

	// log.Printf("%v piece(s)", lPiece)
	// log.Printf("bc: %v", bc)
	// OK. Good to go.
	return map[int]func(){
		0: func() {
			//~ Get length
			req := co.NewPostReqf("/wr/%v", filepath.Base(inpath))
			req.Payload = trait
			sender(req, func(resp *coap.Message) bool {
				if resp.Code == coap.Created && isHexString.MatchString(string(resp.Payload)) {
					shortid = string(resp.Payload)
					log.Printf("I got short-id: %v", shortid)
					proc.KickOff(1)
					return true
				}
				log.Printf("resp-code: %v", resp.Code)
				return false
			})
		},
		1: func() {
			for i := 0; i < bc; i++ {
				doUpload(i)
			}
		},
		2: func() {
			req := co.NewPostReqf("/fin/%v", shortid)
			sender(req, func(resp *coap.Message) bool {
				if resp.Code == coap.Changed {
					log.Printf("fin ok")
					comm.SetExitCode(comm.ExitCode_Success) // mark it as successful.
				} else {
					log.Printf("server fin failed")
				}
				doFinish()
				return true
			})
		},
	}
}
