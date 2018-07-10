package data

import (
	"log"
	"strconv"
	"sync"
)

type composerMan struct {
	composerLock *sync.RWMutex
	compoMap     map[string]*Composer
	cmNameSvr    *NameSvr
	outCh        chan<- *WorkItem
}

func newComposerMan(outCh chan<- *WorkItem) *composerMan {
	return &composerMan{
		composerLock: new(sync.RWMutex),
		compoMap:     make(map[string]*Composer),
		cmNameSvr:    NewNameServer(),
		outCh:        outCh,
	}
}

func (cm *composerMan) getComposerForID(shortname string) (rv *Composer) {
	cm.composerLock.RLock()
	if id, err := strconv.ParseInt(shortname, 16, 64); nil == err {
		if name, err := cm.cmNameSvr.NameForID(int(id)); nil == err {
			rv = cm.compoMap[name]
		} else {
			log.Printf("error id: %v", err)
		}
	} else {
		log.Printf("error parsing 16-based id: %v", err)
	}
	cm.composerLock.RUnlock()
	return
}

func (cm *composerMan) makeComposer(name string, traitBuff []byte) (int, bool) {
	cm.composerLock.Lock()
	defer cm.composerLock.Unlock()
	if _, ok := cm.compoMap[name]; ok {
		// Already exist !
		log.Printf("already exist. you may check it out")
		return 0, false
	}
	newID, err := cm.cmNameSvr.IdForName(name)
	if err != nil {
		log.Printf("error map to id: %v", err)
		return 0, false
	}
	cm.compoMap[name] = NewComposer(name, newID, traitBuff, cm.outCh, func() {
		cm.finalizeComposer(name)
	})
	return newID, true
}

func (cm *composerMan) finalizeComposer(name string) {
	cm.composerLock.Lock()
	if _, ok := cm.compoMap[name]; !ok {
		log.Fatalf("error: %v is not in composer-map", name)
	}
	if cm.cmNameSvr.DeregisterName(name) != nil {
		log.Fatalf("must stay the same as composer-name map")
	}
	delete(cm.compoMap, name)
	cm.composerLock.Unlock()
}

func (cm *composerMan) shutdownComposerMan() {
	var ms []IClean
	cm.composerLock.Lock()
	for _, v := range cm.compoMap {
		ms = append(ms, v)
	}
	cm.composerLock.Unlock()
	for _, v := range ms {
		v.DoClean()
	}
}
