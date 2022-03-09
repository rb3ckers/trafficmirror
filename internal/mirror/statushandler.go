package mirror

import (
	"log"
	"time"

	"github.com/sony/gobreaker"
)

type MirrorStatusHandler func(name string, from, to gobreaker.State)

func RemovingStatusHandler(m *Mirror) MirrorStatusHandler {
	return func(name string, from, to gobreaker.State) {
		switch to {
		case gobreaker.StateOpen:
			if from == gobreaker.StateClosed {
				m.Lock()
				defer m.Unlock()
				m.firstFailureTime = time.Now()

				log.Printf("Temporarily not mirroring to target %s.", name)
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
}

func PersistentStatusHandler(m *Mirror) MirrorStatusHandler {
	return func(name string, from, to gobreaker.State) {
		switch to {
		case gobreaker.StateOpen:
			if from == gobreaker.StateClosed {
				m.Lock()
				defer m.Unlock()
				m.firstFailureTime = time.Now()

				log.Printf("Temporarily not mirroring to target %s.", name)
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
}
