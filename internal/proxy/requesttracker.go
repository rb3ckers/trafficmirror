package proxy

import "sync"

// Track active requests and epoch for new requests
type RequestTracker struct {
	sync.Mutex

	epoch          uint64
	activeRequests map[uint64]interface{}
}

func MakeRequestTracker() *RequestTracker {
	return &RequestTracker{
		epoch:          0,
		activeRequests: make(map[uint64]interface{}, 0),
	}
}

func (t *RequestTracker) NewRequest() (uint64, map[uint64]interface{}) {
	t.Lock()
	defer t.Unlock()

	// Process this under lock. We get the following invariant: a consecutive request cannot be started if an earlier request could not be started
	// from the sendqueue, this is is because consecutive requests will have strictly higher/less active requests
	var currentActive = make(map[uint64]interface{}, len(t.activeRequests))
	for k, _ := range t.activeRequests {
		currentActive[k] = nil
	}

	t.epoch++
	t.activeRequests[t.epoch] = nil

	return t.epoch, currentActive
}

func (t *RequestTracker) RequestDone(epoch uint64) {
	t.Lock()
	defer t.Unlock()

	delete(t.activeRequests, epoch)
}
