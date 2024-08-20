package mirror

import "net/http"

type Request struct {
	originalRequest *http.Request
	body            []byte

	// Used to replay requests in the same order.
	epoch uint64
	// Allow some parallelism based on observed parallelism
	activeRequests map[uint64]interface{}
}

func NewRequest(req *http.Request, body []byte, epoch uint64, activeRequests map[uint64]interface{}) *Request {
	return &Request{
		originalRequest: req,
		body:            body,
		epoch:           epoch,
		activeRequests:  activeRequests,
	}
}
