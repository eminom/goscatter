package data

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"

	"github.com/eminom/go-coap"
)

type ServeMan struct {
	ansCh chan<- *WorkItem

	composerLock *sync.RWMutex
	compoMap     map[string]*Composer
	namesvr      *NameSvr

	// scatterMap map[string]*Scatter
	// scatterLock  *sync.RWMutex
}

func NewServeMan(ansCh chan<- *WorkItem) *ServeMan {
	rv := &ServeMan{
		ansCh: ansCh,

		composerLock: new(sync.RWMutex),
		compoMap:     make(map[string]*Composer),
		namesvr:      NewNameServer(),

		// scatterLock: new(sync.RWMutex),
		// scatterMap:  make(map[string]*Scatter),
	}
	return rv
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
			log.Fatalf("not implmented yet")
			//TODO: initiate a scatter here.
		}
	}

	// fin.
	pushToOutch(resp, from, sm.ansCh)
}

func (sm *ServeMan) replayGeneralError(req coap.Message, from net.Addr) {
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
		sm.replayGeneralError(req, from)
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

func (sm *ServeMan) getComposerForID(shortname string) (rv *Composer) {
	sm.composerLock.RLock()
	if id, err := strconv.ParseInt(shortname, 16, 64); nil == err {
		if name, err := sm.namesvr.NameForID(int(id)); nil == err {
			rv = sm.compoMap[name]
		} else {
			log.Printf("error id: %v", err)
		}
	} else {
		log.Printf("error parsing 16-based id: %v", err)
	}
	sm.composerLock.RUnlock()
	return
}

func (sm *ServeMan) makeComposer(name string, payload []byte) (int, bool) {
	sm.composerLock.Lock()
	defer sm.composerLock.Unlock()
	if _, ok := sm.compoMap[name]; ok {
		// Already exist !
		log.Printf("already exist. you may check it out")
		return 0, false
	}
	newID, err := sm.namesvr.IdForName(name)
	if err != nil {
		log.Printf("error map to id: %v", err)
		return 0, false
	}
	sm.compoMap[name] = NewComposer(name, newID, payload, sm.ansCh, func() {
		sm.finalizeComposer(name)
	})
	return newID, true
}

func (sm *ServeMan) finalizeComposer(name string) {
	sm.composerLock.Lock()
	if _, ok := sm.compoMap[name]; !ok {
		log.Fatalf("error: %v is not in composer-map", name)
	}
	if sm.namesvr.DeregisterName(name) != nil {
		log.Fatalf("must stay the same as composer-name map")
	}
	delete(sm.compoMap, name)
	sm.composerLock.Unlock()
}

//TODO: Processing of scatter-map
