GO_LDFLAGS=-ldflags "-extldflags '-static -pthread' -w"

.PHONY: build restart

default: build

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a \
		${GO_LDFLAGS} \
		-tags netgo \
		-installsuffix netgo \
		-o sse .

restart:
	docker-compose up -d --no-deps --build
