package data

import (
	"log"
	"net"
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
		Code: coap.BadRequest,
	}

	paths := req.Path()
	if len(paths) >= 2 {
		switch paths[0] {
		case "wr":
			//TODO: make a short name for paths[1]
			if sm.makeComposer(paths[1], req.Payload) {
				resp.Code = coap.Created
			}

		case "rd":
			//TODO: initiate a scatter here.
		}
	}

	// fin.
	resp.Type = coap.Acknowledgement
	resp.Token = req.Token
	resp.MessageID = req.MessageID
	pushToOutch(resp, from, sm.ansCh)
}

func (sm *ServeMan) ForwardPut(req *coap.Message, from net.Addr) {

}

func (sm *ServeMan) makeComposer(name string, payload []byte) bool {
	sm.composerLock.Lock()
	defer sm.composerLock.Unlock()
	if _, ok := sm.compoMap[name]; ok {
		// Already exist !
		return false
	}
	sm.compoMap[name] = NewComposer(name, payload, sm.ansCh, func() {
		sm.finalizeComposer(name)
	})
	return true
}

func (sm *ServeMan) finalizeComposer(name string) {
	sm.composerLock.Lock()
	if _, ok := sm.compoMap[name]; !ok {
		log.Fatalf("error: %v is not in composer-map", name)
	}
	delete(sm.compoMap, name)
	sm.composerLock.Unlock()
}

//TODO: Processing of scatter-map
