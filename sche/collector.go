package sche

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	co "../coaputils"
	"../data"

	"github.com/eminom/go-coap"
)

const (
	MinWinSize int32 = 1
	MaxWinSize int32 = 1024
)

type Collector struct {
	WindowSize int

	bombRoad    int32
	segsCount   int
	filename    string
	hmac256     []byte
	sender      func(*coap.Message, func(*coap.Message) bool)
	timeStarted time.Time

	data.FileOpComm
}

func NewCollector(segCount int, filename string, hmac256 []byte,
	sender func(*coap.Message, func(*coap.Message) bool),
) *Collector {
	return &Collector{
		segsCount:   segCount,
		sender:      sender,
		filename:    filename,
		hmac256:     hmac256,
		timeStarted: time.Now(),
	}
}

func (c *Collector) StartCollect(whenDone func()) {
	sender := c.sender

	chunkArr := make([][]byte, c.segsCount)
	doPick := func(data []byte, id int) {
		chunkArr[id] = data
	}

	doSaveToDisk := func() {
		saveTo := filepath.Base(c.filename)

		tmpName := saveTo + ".tmp"
		if c.SaveToFile(tmpName, chunkArr) && c.VerifyFile(tmpName, c.hmac256) && nil == os.Rename(tmpName, saveTo) {
			// log.Printf("hashed verified")
			var totLength int64 = 0
			for _, dat := range chunkArr {
				totLength += int64(len(dat))
			}
			// log.Printf("saved.")
			elapsed := time.Now().Sub(c.timeStarted)
			bandwidth := float64(totLength) / elapsed.Seconds() / 1024
			log.Printf("%.2f kbps", bandwidth)
		} else {
			log.Printf("error hmac of SHA256")
			log.Printf("expecting: %v", hex.EncodeToString(c.hmac256))
		}
		whenDone()
	}

	bc := int32(c.WindowSize)
	if bc < MinWinSize {
		bc = MinWinSize
	} else if bc > MaxWinSize {
		bc = MaxWinSize
	}
	c.bombRoad = bc
	var doRush func(int)
	doRush = func(seqIdx int) {
		if seqIdx >= c.segsCount {
			nuevo := atomic.AddInt32(&c.bombRoad, -1)
			if nuevo == 0 {
				log.Printf("done.")
				doSaveToDisk()
			}
			return
		}
		// log.Printf("req for %v", seqIdx)
		req0 := co.NewGetReq(fmt.Sprintf("/file/%v/0", seqIdx))
		var newPiece data.Piece
		sender(req0, func(resp *coap.Message) bool {
			newPiece.Sig = bytes.Repeat(resp.Payload, 1)
			if newPiece.VerifySig() {
				// log.Printf("sig for %v is ok", seqIdx)
				req1 := co.NewGetReq(fmt.Sprintf("/file/%v/1", seqIdx))
				sender(req1, func(rsp2 *coap.Message) bool {
					newPiece.Chunk = bytes.Repeat(rsp2.Payload, 1)
					if newPiece.VerifyContent() {
						// log.Printf("chunk for %v is ok", seqIdx)
						doPick(newPiece.Chunk, seqIdx)
						doRush(seqIdx + int(bc))
						return true
					}
					return false
				})
				return true
			}
			return false
		})
	}
	for i := 0; i < int(bc); i++ {
		doRush(i)
	}
}

func MakeSacarWork(proc Sche, winSize int, sender func(*coap.Message, func(*coap.Message) bool), doFinish func()) map[int]func() {
	var segs int
	var filename string
	return map[int]func(){
		0: func() {
			//~ Get length
			req := co.NewGetReq("/file/segs")
			sender(req, func(resp *coap.Message) bool {
				var err error
				segs, err = strconv.Atoi(string(resp.Payload))
				if err != nil {
					panic(err)
				}
				log.Printf("seg-count: %v", segs)
				proc.KickOff(1)
				return true
			})
		},
		1: func() {
			req := co.NewGetReq("/file/name")
			sender(req, func(resp *coap.Message) bool {
				filename = string(resp.Payload)
				log.Printf("downloading file: %v", filename)
				proc.KickOff(2)
				return true
			})
		},
		2: func() {
			req := co.NewGetReq("/file/sha256")
			sender(req, func(resp *coap.Message) bool {
				// log.Printf("sha256 for content: %v", hex.EncodeToString(resp.Payload))
				if len(resp.Payload) != sha256.Size {
					panic(fmt.Errorf("fatal error for sha256 request"))
				}
				coll := NewCollector(segs, filename, resp.Payload, sender)
				coll.WindowSize = winSize
				coll.StartCollect(doFinish)
				return true
			})
		},
	}
}
