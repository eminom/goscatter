package data

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eminom/go-coap"
)

type Composer struct {
	oCh chan<- *WorkItem

	name       string
	shortid    int
	hash       []byte // hash for the whole file content
	chunkCount int    // we simply do not store file larger than 2G

	pieces     []Piece // chunk count to chunk-count
	pieceSwap  []Piece
	chunkState []int32
	remains    int

	doStop func()

	heartBeat chan struct{}
	FileOpComm
}

func NewComposer(name string, shortid int, trait []byte, oCh chan<- *WorkItem, whenDone func()) *Composer {
	hash, chunkCount, err := ParseComposerTrait(trait)
	if err != nil {
		log.Printf("error: %v", err)
		return nil
	}
	if chunkCount < 0 {
		log.Printf("error chunk-count: %v", chunkCount)
		return nil
	}

	hb := make(chan struct{}, 1)
	ctx, doCancelSub := context.WithCancel(context.Background())
	doneCh := ctx.Done()

	var unoDo sync.Once
	thisStop := func() {
		unoDo.Do(func() {
			doCancelSub()
			whenDone()
		})
	}

	log.Printf("%v chunk(s) for %v", chunkCount, name)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-doneCh:
				return
			case <-time.After(15 * time.Second):
				log.Printf("timeout for this composer: %v(short-id:%v)", name, shortid)
				thisStop() // timeout !!!
				return
			case <-hb:
				// ok, good to go
			}
		}
	}()

	// Pay attention:
	// At the beginnin, I forgot assigning oCh, which left chan of value `nil'
	// But it is ok to push something onto the nil channel.
	// No warning or panic is issued.
	// Please be very care to test every blocking channel

	return &Composer{
		oCh:        oCh,
		name:       name,
		shortid:    shortid,
		hash:       hash,
		chunkCount: chunkCount,
		remains:    chunkCount,
		pieces:     make([]Piece, chunkCount),
		pieceSwap:  make([]Piece, chunkCount),
		chunkState: make([]int32, chunkCount),
		doStop:     thisStop,
		heartBeat:  hb,
	}
}

func (c *Composer) DoFinish() bool {
	defer c.doStop()
	if 0 == c.remains {
		tmpName := c.name + ".tmp"
		if c.SaveToFile(tmpName, c.PiecesToArray(c.pieces)) {
			log.Printf("verifying for <%v>...", c.name)
			if c.VerifyFile(tmpName, c.hash) {
				if nil == os.Rename(tmpName, c.name) {
					log.Printf("<%v> is stored.", c.name)
					return true
				}
			}
		}
	}
	return false
}

func (c *Composer) GetName() string {
	return c.name
}

func (c *Composer) SoakUp(idxstr string, sel string, req *coap.Message, from net.Addr) {

	var resp = &coap.Message{
		Code:      coap.BadRequest,
		Type:      coap.Acknowledgement,
		MessageID: req.MessageID,
		Token:     req.Token,
	}

	if idx, err := strconv.Atoi(idxstr); nil == err {
		switch sel {
		default:
			log.Printf("error sel: %v", sel)
		case "0":
			if c.sinkSig(int(idx), req.Payload) {
				resp.Code = coap.Changed
			}
		case "1":
			if c.sinkChunk(int(idx), req.Payload) {
				resp.Code = coap.Changed
			}
		}
	}

	c.heartBeat <- struct{}{}
	pushToOutch(resp, from, c.oCh)
}

// can be access simultaneously.
func (c *Composer) sinkSig(idx int, sig []byte) bool {
	if idx < 0 || idx >= c.chunkCount {
		log.Printf("error index number for signature(upload):%v", idx)
		return false
	}
	c.pieceSwap[idx].Sig = sig
	rv := c.pieceSwap[idx].VerifySig()
	if rv {
		c.pieces[idx].Sig = sig
	}
	return rv
}

// can be access simultaneously.
func (c *Composer) sinkChunk(idx int, chunk []byte) bool {
	if idx < 0 || idx >= c.chunkCount {
		log.Printf("error chunk index: %v", idx)
		return false
	}
	c.pieceSwap[idx].Chunk = chunk
	isVerified := c.pieceSwap[idx].VerifyContent()
	if isVerified && atomic.CompareAndSwapInt32(&c.chunkState[idx], 0, 1) {
		c.remains -= 1
		c.pieces[idx].Chunk = chunk
	}
	return isVerified
}
