package datatypes

import (
	"log"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

// MirrorTargets has the set of target URLs to mirror to
// It protects these for multi threaded access
type MirrorTargets struct {
	sync.RWMutex
	targets                  map[string]*targetState
	persistentFailureTimeout time.Duration
	retryAfter               time.Duration
}

type MirrorSettings struct {
	PersistentFailureTimeout time.Duration
	RetryAfter               time.Duration
}

type Target struct {
	Name         string
	FailingSince time.Time
	State        string
}

type targetState struct {
	sync.Mutex
	firstFailure             time.Time
	persistentFailureTimeout time.Duration
	circuitBreaker           *gobreaker.CircuitBreaker
	onTargetFailed           func(target string)
}

func NewMirrorTargets(settings MirrorSettings) *MirrorTargets {
	return &MirrorTargets{
		targets:                  make(map[string]*targetState),
		persistentFailureTimeout: settings.PersistentFailureTimeout,
		retryAfter:               settings.RetryAfter,
	}
}

func (mt *MirrorTargets) Add(targets []string) {
	log.Printf("Adding %s to targets list.", targets)
	mt.Lock()
	defer mt.Unlock()
	for _, url := range targets {
		mt.targets[url] = newTargetState(url, mt.persistentFailureTimeout, mt.retryAfter, func(target string) {
			mt.Delete([]string{target})
		})
	}
}

func (mt *MirrorTargets) Delete(targets []string) {
	log.Printf("Removing %s from targets list.", targets)
	mt.Lock()
	defer mt.Unlock()
	for _, url := range targets {
		delete(mt.targets, url)
	}
}

func (mt *MirrorTargets) ForEach(f func(string, *gobreaker.CircuitBreaker)) {
	mt.RLock()
	defer mt.RUnlock()
	for url, target := range mt.targets {
		f(url, target.circuitBreaker)
	}
}

func (mt *MirrorTargets) ListTargets() []*Target {
	targets := make([]*Target, len(mt.targets))
	i := 0
	for url, target := range mt.targets {
		var state string
		switch target.circuitBreaker.State() {
		case gobreaker.StateOpen:
			state = "failing"
		case gobreaker.StateHalfOpen:
			state = "retrying"
		case gobreaker.StateClosed:
			state = "alive"
		default:
			state = "unknown"
		}

		targets[i] = &Target{
			Name:         url,
			FailingSince: target.firstFailure,
			State:        state,
		}
		i = i + 1
	}
	return targets
}

func (ts *targetState) onBreakerChange(name string, from gobreaker.State, to gobreaker.State) {
	if from == gobreaker.StateClosed && to == gobreaker.StateOpen {
		ts.Lock()
		defer ts.Unlock()
		ts.firstFailure = time.Now()
		log.Printf("Temporary not mirroring to target %s.", name)
	} else if to == gobreaker.StateOpen {
		ts.Lock()
		defer ts.Unlock()
		if !ts.firstFailure.IsZero() && time.Now().Sub(ts.firstFailure) > ts.persistentFailureTimeout {
			log.Printf("%s is persistently failing.", name)
			ts.onTargetFailed(name)
		}
	} else if to == gobreaker.StateHalfOpen {
		log.Printf("Retrying target %s.", name)
	} else if to == gobreaker.StateClosed {
		ts.Lock()
		defer ts.Unlock()
		ts.firstFailure = time.Time{}
		log.Printf("Resuming mirroring to target %s.", name)
	}
}

func newTargetState(name string, persistentFailureTimeout time.Duration, retryAfter time.Duration, onTargetFailed func(target string)) *targetState {
	targetState := &targetState{
		circuitBreaker:           nil,
		onTargetFailed:           onTargetFailed,
		persistentFailureTimeout: persistentFailureTimeout,
	}
	settings := gobreaker.Settings{
		Name:          name,
		MaxRequests:   1,
		Interval:      0,          // Never clear counts
		Timeout:       retryAfter, // When open retry after 60 seconds
		OnStateChange: targetState.onBreakerChange,
	}
	targetState.circuitBreaker = gobreaker.NewCircuitBreaker(settings)
	return targetState
}
