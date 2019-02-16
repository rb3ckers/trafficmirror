package datatypes

import "sync"

// MirrorTargets has the set of target URLs to mirror to
// It protects these for multi threaded access
type MirrorTargets struct {
	sync.RWMutex
	targets map[string]bool
}

func NewMirrorTargets() *MirrorTargets {
	return &MirrorTargets{
		targets: make(map[string]bool),
	}
}

func (me *MirrorTargets) Add(targets []string) {
	me.Lock()
	for _, url := range targets {
		me.targets[url] = true
	}
	me.Unlock()
}

func (me *MirrorTargets) Delete(targets []string) {
	me.Lock()
	for _, url := range targets {
		delete(me.targets, url)
	}
	me.Unlock()
}

func (me *MirrorTargets) ForEach(f func(string)) {
	me.RLock()
	for url := range me.targets {
		f(url)
	}
	me.RUnlock()
}
