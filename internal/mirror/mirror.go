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

	"github.com/rb3ckers/trafficmirror/internal/config"
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
	sendQueue                *SendQueue
}

type MirrorState string

var (
	StateFailing  MirrorState = "failing"
	StateRetrying MirrorState = "retrying"
	StateAlive    MirrorState = "alive"
	StateUnkown   MirrorState = "unknown"
)

type MirrorStatus struct {
	State          MirrorState
	FailingSince   time.Time
	URL            string
	QueuedRequests int
	Epoch          uint64
}

func NewMirror(targetURL string, config *config.Config, failureCh chan<- string, persistent bool, sendQueue *SendQueue) *Mirror {
	retryAfter := time.Duration(config.RetryAfter) * time.Minute
	persistentFailureTimeout := time.Duration(config.PersistentFailureTimeout) * time.Minute

	mirror := &Mirror{
		netClient: &http.Client{
			Timeout: time.Second * 20,
		},
		persistentFailureTimeout: persistentFailureTimeout,
		targetURL:                targetURL,
		failureCh:                failureCh,
		sendQueue:                sendQueue,
	}

	settings := gobreaker.Settings{
		Name:        targetURL,
		MaxRequests: 1,
		Interval:    0,          // Never clear counts
		Timeout:     retryAfter, // When open retry after 60 seconds
	}

	if persistent {
		settings.OnStateChange = PersistentStatusHandler(mirror)
	} else {
		settings.OnStateChange = RemovingStatusHandler(mirror)
	}

	breaker := gobreaker.NewCircuitBreaker(settings)

	mirror.breaker = breaker

	return mirror
}

func (m *Mirror) Reflect(req *Request) {
	m.sendQueue.AddToQueue(req, m.targetURL)
	// Attempt sending the next items
	m.tryExecuteNext()
}

func (m *Mirror) tryExecuteNext() {
	for _, r := range m.sendQueue.NextExecuteItems() {
		go m.executeRequest(r)
	}
}

func (m *Mirror) executeRequest(req *Request) {
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

	m.sendQueue.ExecutionCompleted(req)
	m.tryExecuteNext()
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

	epoch, queued := m.sendQueue.QueueStatus()

	return &MirrorStatus{
		State:          state,
		FailingSince:   m.firstFailureTime,
		URL:            m.targetURL,
		QueuedRequests: queued,
		Epoch:          epoch,
	}
}
