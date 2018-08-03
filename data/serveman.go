package data

import (
	"fmt"
	"log"
	"net"
	"strconv"

	"../comm"

	"github.com/eminom/go-coap"
)

const (
	SpecialDirName     = "<?>"
	DefaultSegmentSize = 512
	MaxSegmentSize     = 64 * 1024
)

func IsSpecialDirName(name string) bool {
	return name == SpecialDirName
}

type ServeMan struct {
	ansCh chan<- *WorkItem
	// scatterMap map[string]*Scatter
	// scatterLock  *sync.RWMutex

	*composerMan
	*scatterMan
}

func NewServeMan(ansCh chan<- *WorkItem) *ServeMan {
	rv := &ServeMan{
		ansCh:       ansCh,
		composerMan: newComposerMan(ansCh),
		scatterMan:  newScatterMan(ansCh),

		// scatterLock: new(sync.RWMutex),
		// scatterMap:  make(map[string]*Scatter),
	}
	return rv
}

func (sm *ServeMan) Close() {
	sm.shutdownComposerMan()
	sm.shutdownScatterMan()
}

func (sm *ServeMan) ProcessPost(req coap.Message, from net.Addr) {
	resp := &coap.Message{
		Code:      coap.BadRequest,
		Type:      coap.Acknowledgement,
		Token:     req.Token,
		MessageID: req.MessageID,
	}

	paths := req.Path()
	if len(paths) >= 2 {
		switch paths[0] {
		case "fin":
			if composer := sm.getComposerForID(paths[1]); composer != nil {
				if composer.DoFinish() {
					resp.Code = coap.Changed
				} else {
					resp.Code = coap.InternalServerError
				}
			} else {
				log.Printf("not such composer: <%v>", paths[1])
				resp.Type = coap.Reset // make it harder.
			}

		case "wr":
			if thisID, ok := sm.makeComposer(paths[1], req.Payload); ok {
				resp.Code = coap.Created
				resp.Payload = []byte(fmt.Sprintf("%02x", thisID))
			}

		case "rd":
			reqFileName := string(req.Payload)
			var proposedSegmentSize int = DefaultSegmentSize
			if sizeVal, err := strconv.Atoi(paths[1]); nil == err {
				if 0 < sizeVal && sizeVal <= MaxSegmentSize {
					proposedSegmentSize = sizeVal
				}
			}
			if IsSpecialDirName(reqFileName) {
				resp.Code = coap.Created
				if thisID, thatSegSize, ok := sm.findScatter(reqFileName); ok {
					resp.Payload = []byte(fmt.Sprintf("%02x,%v", thisID, thatSegSize))
				} else {
					resp.Type = coap.Reset //or error
					resp.Payload = []byte(fmt.Sprintf("no such query"))
				}
			} else if !comm.IsFileExists(reqFileName) {
				resp.Type = coap.Reset
				resp.Payload = []byte(fmt.Sprintf("no such file: %v", string(req.Payload)))
			} else if thisID, thatSegSize, ok := sm.makeScatter(proposedSegmentSize, reqFileName); ok {
				resp.Code = coap.Created
				resp.Payload = []byte(fmt.Sprintf("%02x,%v", thisID, thatSegSize))
			}

		case "done":
			if scatter := sm.getScatterForID(paths[1]); scatter != nil {
				scatter.DoStop()
				resp.Code = coap.Changed
			} else {
				log.Printf("no such scatter: <%v>", paths[1])
				resp.Type = coap.Reset
			}
		}
	}

	// fin.
	pushToOutch(resp, from, sm.ansCh)
}

func (sm *ServeMan) replyGeneralError(req coap.Message, from net.Addr) {
	pushToOutch(&coap.Message{
		Token:     req.Token,
		MessageID: req.MessageID,
		Type:      coap.Acknowledgement,
		Payload:   []byte("general error"),
	}, from, sm.ansCh)
}

func (sm *ServeMan) replayErrorWithReset(req coap.Message, from net.Addr, msg string) {
	pushToOutch(&coap.Message{
		Token:     req.Token,
		MessageID: req.MessageID,
		Type:      coap.Reset, // This is very fatal very much
		Payload:   []byte(msg),
	}, from, sm.ansCh)

}

// Uri-Path:  /f/short-id/index-of-chunk/select(0/1)
// length: 4
func (sm *ServeMan) ForwardPut(req coap.Message, from net.Addr) {
	paths := req.Path()
	if len(paths) <= 0 {
		log.Printf("error path for put")
		sm.replyGeneralError(req, from)
		return
	}
	switch paths[0] {
	case "f":
		if len(paths) == 4 {
			c := sm.getComposerForID(paths[1])
			if c != nil {
				c.SoakUp(paths[2], paths[3], &req, from)
			} else {
				sm.replayErrorWithReset(req, from, "no such composer")
			}
		}
	default:
		log.Printf("unknown protocol: <%v>", paths[0])
	}
}

// Uri-Path: /f/short-id/index-of-chunk/select(0/1)
func (sm *ServeMan) ForwardGet(req coap.Message, from net.Addr) {
	paths := req.Path()
	if len(paths) <= 0 {
		log.Printf("error path for get")
		sm.replyGeneralError(req, from)
		return
	}
	switch paths[0] {
	case "f":
		for {
			if len(paths) >= 2 {
				s := sm.getScatterForID(paths[1])
				if s != nil {
					s.RipUp(paths[2:], &req, from)
					break
				}
			}
			sm.replayErrorWithReset(req, from, "no such scatter")
			break
		}
	default:
		log.Printf("unknown protocal: <%v>", paths[0])
	}

}
