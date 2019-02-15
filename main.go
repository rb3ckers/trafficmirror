package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

func bufferRequest(req *http.Request) []byte {
	// Read body to buffer
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
	}

	// Restore the body so we can send it multiple times
	req.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	return body
}

var netClient = &http.Client{
	Timeout: time.Second * 10,
}

func mirrorTo(targetURL string, req *http.Request, body []byte) {
	url := fmt.Sprintf("%s%s", targetURL, req.RequestURI)

	newRequest, _ := http.NewRequest(req.Method, url, bytes.NewReader(body))
	newRequest.Header = req.Header

	response, err := netClient.Do(newRequest)
	if err != nil {
		log.Printf("Error reading response: %v", err)
		return
	}
	defer response.Body.Close()
}

func main() {
	listenPort := flag.String("port", "7071", "Port to listen on")
	proxyTarget := flag.String("main", "http://localhost:7072", "Main proxy target, its responses will be returned")
	help := flag.Bool("help", false, "Print help")

	flag.Parse()

	if *help {
		flag.PrintDefaults()
		return
	}

	url, _ := url.Parse(*proxyTarget)

	mirrorURL := "http://localhost:7073"

	proxyTo := httputil.NewSingleHostReverseProxy(url)

	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		body := bufferRequest(req)

		proxyTo.ServeHTTP(res, req)
		go mirrorTo(mirrorURL, req, body)
	})

	// start server
	if err := http.ListenAndServe(":"+*listenPort, nil); err != nil {
		panic(err)
	}
}
