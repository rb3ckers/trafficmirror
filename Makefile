SHELL := /bin/bash

clean:
	rm -rf build

build:
	source build-env.sh && \
	cd $(GO_PROJECT_PATH) && \
	dep ensure && \
	mkdir -p build && \
	GOOS=linux GOARCH=amd64 go build -o build/trafficmirror
