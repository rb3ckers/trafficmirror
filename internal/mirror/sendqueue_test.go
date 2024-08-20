package mirror

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func mkRequest(epoch uint64, active []uint64) *Request {
	var activeMap map[uint64]interface{} = make(map[uint64]interface{}, 0)

	for _, activeEpoch := range active {
		activeMap[activeEpoch] = nil
	}

	return &Request{
		originalRequest: nil,
		body:            nil,
		epoch:           epoch,
		activeRequests:  activeMap,
	}
}

func TestSendQueueSendsInOrder(t *testing.T) {
	q := MakeSendQueue(5)

	r1 := mkRequest(1, []uint64{})
	r2 := mkRequest(2, []uint64{})
	r3 := mkRequest(3, []uint64{})
	r4 := mkRequest(4, []uint64{})

	q.AddToQueue(r3, "url")
	q.AddToQueue(r1, "url")
	q.AddToQueue(r4, "url")
	q.AddToQueue(r2, "url")

	var expectNil []*Request
	// All items in order

	assert.Equal(t, []*Request{r1}, q.NextExecuteItems())
	assert.Equal(t, expectNil, q.NextExecuteItems())
	q.ExecutionCompleted(r1)

	assert.Equal(t, []*Request{r2}, q.NextExecuteItems())
	assert.Equal(t, expectNil, q.NextExecuteItems())
	q.ExecutionCompleted(r2)

	assert.Equal(t, []*Request{r3}, q.NextExecuteItems())
	assert.Equal(t, expectNil, q.NextExecuteItems())
	q.ExecutionCompleted(r3)

	assert.Equal(t, []*Request{r4}, q.NextExecuteItems())
	assert.Equal(t, expectNil, q.NextExecuteItems())
	q.ExecutionCompleted(r4)

	// Some whitebox testing
	assert.Equal(t, uint64(4), q.completedEpochsUntil)
	assert.Equal(t, make(map[uint64]interface{}, 0), q.epochsCompleted)
	assert.Equal(t, expectNil, q.requestsQueued)
}

func TestSendParallelWhenPartOfActive(t *testing.T) {
	q := MakeSendQueue(5)

	r1 := mkRequest(1, []uint64{})
	r2 := mkRequest(2, []uint64{1})

	q.AddToQueue(r1, "url")
	q.AddToQueue(r2, "url")

	var expectNil []*Request

	assert.Equal(t, []*Request{r1, r2}, q.NextExecuteItems())
	assert.Equal(t, expectNil, q.NextExecuteItems())
}

func TestExecutedWhenActiveAndCompleted(t *testing.T) {
	q := MakeSendQueue(5)

	r1 := mkRequest(1, []uint64{})
	r2 := mkRequest(2, []uint64{1})
	r3 := mkRequest(3, []uint64{1})

	q.AddToQueue(r2, "url")
	q.AddToQueue(r3, "url")

	var expectNil []*Request

	assert.Equal(t, []*Request{r2}, q.NextExecuteItems())
	assert.Equal(t, expectNil, q.NextExecuteItems())
	// Complete 2, this also allows 3 to be executed
	q.ExecutionCompleted(r2)

	// Complete 3
	assert.Equal(t, []*Request{r3}, q.NextExecuteItems())
	assert.Equal(t, expectNil, q.NextExecuteItems())
	q.ExecutionCompleted(r3)
	assert.Equal(t, expectNil, q.NextExecuteItems())

	// Some whitebox testing on the epochs tracking
	var expectCompleted = make(map[uint64]interface{}, 0)
	expectCompleted[2] = nil
	expectCompleted[3] = nil
	assert.Equal(t, uint64(0), q.completedEpochsUntil)
	assert.Equal(t, expectCompleted, q.epochsCompleted)
	assert.Equal(t, expectNil, q.requestsQueued)

	// Now run r1
	q.AddToQueue(r1, "url")
	assert.Equal(t, []*Request{r1}, q.NextExecuteItems())
	assert.Equal(t, expectNil, q.NextExecuteItems())
	q.ExecutionCompleted(r1)

	assert.Equal(t, uint64(3), q.completedEpochsUntil)
	assert.Equal(t, make(map[uint64]interface{}, 0), q.epochsCompleted)
	assert.Equal(t, expectNil, q.requestsQueued)
}

func TestLimitQueueSize(t *testing.T) {
	q := MakeSendQueue(1)

	r1 := mkRequest(1, []uint64{})
	r2 := mkRequest(2, []uint64{1})

	q.AddToQueue(r1, "url")
	q.AddToQueue(r2, "url")

	var expectNil []*Request

	assert.Equal(t, []*Request{r1}, q.NextExecuteItems())
	assert.Equal(t, expectNil, q.NextExecuteItems())
	q.ExecutionCompleted(r1)

	assert.Equal(t, uint64(2), q.completedEpochsUntil)
	assert.Equal(t, make(map[uint64]interface{}, 0), q.epochsCompleted)
	assert.Equal(t, expectNil, q.requestsQueued)
}
