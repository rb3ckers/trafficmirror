package mirror

import (
	"log"
	"sync"

	"github.com/rb3ckers/trafficmirror/internal/config"
)

type Reflector struct {
	sync.RWMutex
	mirrors           map[string]*Mirror
	IncomingCh        chan *Request
	DoneCh            chan bool
	MirrorFailureChan chan string
	config            *config.Config
	// This sendQueue is kept to keep exact state of what epochs were sent by the handler. This is used when we make a
	// new mirror so we the state of that new mirror is in sync.
	templateSendQueue *SendQueue
}

func NewReflector(config *config.Config) *Reflector {
	return &Reflector{
		mirrors:           make(map[string]*Mirror),
		IncomingCh:        make(chan *Request),
		DoneCh:            make(chan bool),
		MirrorFailureChan: make(chan string),
		config:            config,
		templateSendQueue: MakeSendQueue(config.MaxQueuedRequests),
	}
}

func (r *Reflector) Reflect() {
	log.Printf("Reflector started.")

	for {
		select {
		case req := <-r.IncomingCh:
			r.updateTemplateQueue(req)
			r.sendToMirrors(req)
		case url := <-r.MirrorFailureChan:
			log.Printf("Mirror '%s' has persistent failures", url)
			r.RemoveMirrors([]string{url})
		case <-r.DoneCh:
			return
		}
	}
}

func (r *Reflector) updateTemplateQueue(req *Request) {
	// Update the
	r.templateSendQueue.AddToQueue(req, "template")
	// Execute all possible items
	for {
		requests := r.templateSendQueue.NextExecuteItems()
		if len(requests) > 0 {
			for _, req := range requests {
				r.templateSendQueue.ExecutionCompleted(req)
			}
		} else {
			break
		}
	}
}

func (r *Reflector) sendToMirrors(req *Request) {
	r.RLock()
	defer r.RUnlock()

	for _, mirror := range r.mirrors {
		go mirror.Reflect(req)
	}
}

func (r *Reflector) AddMirrors(urls []string, persistent bool) {
	r.Lock()
	defer r.Unlock()

	for _, url := range urls {
		log.Printf("Adding '%s' to mirror list.", url)
		r.mirrors[url] = NewMirror(url, r.config, r.MirrorFailureChan, persistent, r.templateSendQueue)
	}
}

func (r *Reflector) RemoveMirrors(urls []string) {
	log.Printf("Removing '%s' from mirror list.", urls)
	r.Lock()
	defer r.Unlock()

	for _, url := range urls {
		delete(r.mirrors, url)
	}
}

func (r *Reflector) ListMirrors() []*MirrorStatus {
	targets := make([]*MirrorStatus, len(r.mirrors))
	i := 0

	for _, target := range r.mirrors {
		targets[i] = target.GetStatus()
		i++
	}

	return targets
}

func (r *Reflector) Close() {
	r.DoneCh <- true
}
