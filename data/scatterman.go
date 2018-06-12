package data

import (
	"log"
	"strconv"
	"sync"
)

type scatterMan struct {
	smLock     *sync.RWMutex
	scatterMap map[string]*Scatter
	smNameSvr  *NameSvr
	smoCh      chan<- *WorkItem
}

func newScatterMan(outCh chan<- *WorkItem) *scatterMan {
	return &scatterMan{
		smLock:     new(sync.RWMutex),
		scatterMap: make(map[string]*Scatter),
		smNameSvr:  NewNameServer(),
		smoCh:      outCh,
	}
}

func (sm *scatterMan) getScatterForID(shortname string) (rv *Scatter) {
	sm.smLock.RLock()
	if id, err := strconv.ParseInt(shortname, 16, 64); nil == err {
		if name, err := sm.smNameSvr.NameForID(int(id)); nil == err {
			rv = sm.scatterMap[name]
		} else {
			log.Printf("error id: %v", err)
		}
	} else {
		log.Printf("error parsing 16-based id: %v", err)
	}
	sm.smLock.RUnlock()
	return
}

func (sm *scatterMan) makeScatter(name string) (int, bool) {

	sm.smLock.Lock()
	defer sm.smLock.Unlock()
	// if _, ok := sm.scatterMap[name]; ok {
	// 	// Already exist !
	// 	log.Printf("already exist. you may check it out")
	// 	return 0, false
	// }
	newID, err := sm.smNameSvr.IdForName(name)
	if err != nil {
		log.Printf("error map to id: %v", err)
		return 0, false
	}
	sm.scatterMap[name] = NewScatter(name, newID, sm.smoCh, func() {
		sm.finalizeScatter(name)
	})
	return newID, true
}

func (sm *scatterMan) finalizeScatter(name string) {
	sm.smLock.Lock()
	if _, ok := sm.scatterMap[name]; !ok {
		log.Fatalf("error: %v is not in composer-map", name)
	}
	if sm.smNameSvr.DeregisterName(name) != nil {
		log.Fatalf("must stay the same as composer-name map")
	}
	delete(sm.scatterMap, name)
	sm.smLock.Unlock()
}
