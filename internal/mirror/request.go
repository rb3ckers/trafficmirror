package mirror

import "net/http"

type Request struct {
	originalRequest *http.Request
	body            []byte
}

func NewRequest(req *http.Request, body []byte) *Request {
	return &Request{
		originalRequest: req,
		body:            body,
	}
}
