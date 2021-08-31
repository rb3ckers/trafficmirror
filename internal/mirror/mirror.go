package mirror

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

type Mirror struct {
	sync.Mutex
	netClient                *http.Client
	targetURL                string
	breaker                  *gobreaker.CircuitBreaker
	firstFailureTime         time.Time
	persistentFailureTimeout time.Duration
	failureCh                chan<- string
}

type MirrorState string

var (
	StateFailing  MirrorState = "failing"
	StateRetrying MirrorState = "retrying"
	StateAlive    MirrorState = "alive"
	StateUnkown   MirrorState = "unknown"
)

type MirrorStatus struct {
	State        MirrorState
	FailingSince time.Time
	URL          string
}

func NewMirror(targetURL string, retryAfter time.Duration, persistentFailureTimeout time.Duration, failureCh chan<- string) *Mirror {
	mirror := &Mirror{
		netClient: &http.Client{
			Timeout: time.Second * 20,
		},
		persistentFailureTimeout: persistentFailureTimeout,
		targetURL:                targetURL,
		failureCh:                failureCh,
	}

	settings := gobreaker.Settings{
		Name:          targetURL,
		MaxRequests:   1,
		Interval:      0,          // Never clear counts
		Timeout:       retryAfter, // When open retry after 60 seconds
		OnStateChange: mirror.onBreakerChange,
	}

	breaker := gobreaker.NewCircuitBreaker(settings)

	mirror.breaker = breaker

	return mirror
}

func (m *Mirror) onBreakerChange(name string, from gobreaker.State, to gobreaker.State) {
	switch to {
	case gobreaker.StateOpen:
		if from == gobreaker.StateClosed {
			m.Lock()
			defer m.Unlock()
			m.firstFailureTime = time.Now()

			log.Printf("Temporary not mirroring to target %s.", name)
		} else {
			m.Lock()
			defer m.Unlock()
			if !m.firstFailureTime.IsZero() && time.Since(m.firstFailureTime) > m.persistentFailureTimeout {
				log.Printf("%s is persistently failing.", name)
				m.failureCh <- m.targetURL
			}
		}
	case gobreaker.StateHalfOpen:
		log.Printf("Retrying target %s.", name)

	case gobreaker.StateClosed:
		m.Lock()
		defer m.Unlock()
		m.firstFailureTime = time.Time{}

		log.Printf("Resuming mirroring to target %s.", name)
	}
}

func (m *Mirror) Reflect(req *Request) {
	m.breaker.Execute(func() (interface{}, error) { //nolint:errcheck
		url := fmt.Sprintf("%s%s", m.targetURL, req.originalRequest.RequestURI)

		newRequest, err := http.NewRequest(req.originalRequest.Method, url, bytes.NewReader(req.body)) //nolint:noctx
		if err != nil {
			return nil, err
		}

		newRequest.Header = req.originalRequest.Header

		response, err := m.netClient.Do(newRequest)
		if err != nil {
			log.Printf("Error reading response: %v", err)
			return nil, err
		}
		defer response.Body.Close()
		// Drain the body, but discard it, to make sure connection can be reused
		return io.Copy(ioutil.Discard, response.Body)
	})
}

func (m *Mirror) GetStatus() *MirrorStatus {
	var state MirrorState

	switch m.breaker.State() {
	case gobreaker.StateOpen:
		state = StateFailing
	case gobreaker.StateHalfOpen:
		state = StateRetrying
	case gobreaker.StateClosed:
		state = StateAlive
	default:
		state = StateUnkown
	}

	return &MirrorStatus{
		State:        state,
		FailingSince: m.firstFailureTime,
		URL:          m.targetURL,
	}
}
