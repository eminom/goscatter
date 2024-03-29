package sche

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"

	co "github.com/eminom/goscatter/coaputils"
	"github.com/eminom/goscatter/comm"
	"github.com/eminom/goscatter/data"

	"github.com/eminom/go-coap"
)

var (
	fVerbose = flag.Bool("v", false, "verbose for sacar")
	fSubpath = flag.Bool("r", false, "store to its sub path")
	fOutpath = flag.String("o", "", "directory prefix")
)

const (
	MinWinSize int32 = 1
	MaxWinSize int32 = 1024
)

var (
	reShortIDSegsize = regexp.MustCompile(`([A-Fa-f\d]+),\s*(\d+)`)
)

var (
	invalidReFormat = errors.New("error format for short-id and segment-size")
)

func parseShortIDndSeg(str string) (shortID int, segSize int, err error) {
	match := reShortIDSegsize.FindStringSubmatch(str)
	if len(match) != 3 {
		return 0, 0, invalidReFormat
	}
	var i64 int64
	i64, err = strconv.ParseInt(match[1], 16, 32)
	if err != nil {
		return 0, 0, err
	}
	shortID = int(i64)
	segSize, err = strconv.Atoi(match[2])
	return
}

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

func (c *Collector) showStatus(chunks [][]byte) {
	var totLength int64 = 0
	for _, dat := range chunks {
		totLength += int64(len(dat))
	}
	// log.Printf("saved.")
	elapsed := time.Now().Sub(c.timeStarted)
	bandwidth := float64(totLength) / elapsed.Seconds() / 1024
	log.Printf("%v elapsed", elapsed)
	log.Printf("%.2f kbps", bandwidth)
}

func (c *Collector) StartCollect(fileShortID int, whenDone func(), verbose bool) {
	sender := c.sender

	chunkArr := make([][]byte, c.segsCount)
	doPick := func(data []byte, id int) {
		chunkArr[id] = data
	}

	doSaveToDisk := func() {
		if data.IsSpecialDirName(c.filename) {
			if !c.VerifyChunk(chunkArr, c.hmac256) {
				log.Printf("list dir error: hash failed")
			} else {
				c.showStatus(chunkArr)
				for _, c := range chunkArr {
					os.Stdout.Write(c)
				}
			}
		} else {
			var saveTo string
			if *fSubpath {
				// use original relative path
				saveTo = c.filename
			} else {
				// base name is good enough
				saveTo = filepath.Base(c.filename)
			}

			// empty path will be ignore.
			saveTo = filepath.Join(*fOutpath, saveTo)
			os.MkdirAll(filepath.Dir(saveTo), os.ModePerm)

			tmpName := saveTo + ".tmp"
			if c.SaveToFile(tmpName, chunkArr) && c.VerifyFile(tmpName, c.hmac256) && nil == os.Rename(tmpName, saveTo) {
				// log.Printf("hashed verified")
				c.showStatus(chunkArr)
			} else {
				log.Printf("error hmac of SHA256")
				log.Printf("expecting: %v", hex.EncodeToString(c.hmac256))
			}
		}

		// and finally.
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

		if verbose {
			log.Printf("req for %v", seqIdx)
		}
		req0 := co.NewGetReqf("/f/%v/%v/0", fileShortID, seqIdx)
		var newPiece data.Piece
		sender(req0, func(resp *coap.Message) bool {
			newPiece.Sig = bytes.Repeat(resp.Payload, 1)
			if newPiece.VerifySig() {
				// log.Printf("sig for %v is ok", seqIdx)
				req1 := co.NewGetReqf("/f/%v/%v/1", fileShortID, seqIdx)
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

func MakeSacarWork(proc Sche, filename string,
	winSize int, fragmentSize int, sender func(*coap.Message, func(*coap.Message) bool), doFinish func()) map[int]func() {
	var segs int
	var shortID int
	var thatSegsize int
	return map[int]func(){
		0: func() {
			req := co.NewPostReqf("/rd/%v", fragmentSize)
			log.Printf("requesting for %v", filename)
			req.Payload = []byte(filename)
			sender(req, func(resp *coap.Message) bool {
				if coap.Acknowledgement == resp.Type && resp.Code == coap.Created {
					var err error
					shortID, thatSegsize, err = parseShortIDndSeg(string(resp.Payload))
					_ = thatSegsize
					if err != nil {
						panic(err)
					}
					log.Printf("read short-id: %v", shortID)
					proc.KickOff(1)
					return true
				}
				log.Printf("rd failed for<%v>:%v", resp.Code, string(resp.Payload))
				doFinish() // fail and quit.
				comm.SetExitCode(comm.ExitCode_NoSuchFile)
				return true
			})
		},
		1: func() {
			//~ Get length
			req := co.NewGetReqf("/f/%v/segs", shortID)
			sender(req, func(resp *coap.Message) bool {
				var err error
				segs, err = strconv.Atoi(string(resp.Payload))
				if err != nil {
					panic(err)
				}
				log.Printf("seg-count: %v", segs)
				proc.KickOff(2)
				return true
			})
		},
		2: func() {
			req := co.NewGetReqf("/f/%v/sha256", shortID)
			sender(req, func(resp *coap.Message) bool {
				// log.Printf("sha256 for content: %v", hex.EncodeToString(resp.Payload))
				if len(resp.Payload) != sha256.Size {
					panic(fmt.Errorf("fatal error for sha256 request"))
				}
				coll := NewCollector(segs, filename, resp.Payload, sender)
				coll.WindowSize = winSize
				coll.StartCollect(shortID, func() {
					proc.KickOff(3)
				}, *fVerbose)
				return true
			})
		},
		3: func() {
			log.Printf("sending done to server ...")
			req := co.NewPostReqf("/done/%v", shortID)
			sender(req, func(resp *coap.Message) bool {
				if coap.Acknowledgement == resp.Type && resp.Code == coap.Changed {
					log.Printf("server side finish ok")
					comm.SetExitCode(comm.ExitCode_Success)
				} else {
					log.Printf("server side finish error:%v - %v", resp.Type, resp.Code)
				}
				doFinish()
				return true
			})
		},
	}

}
