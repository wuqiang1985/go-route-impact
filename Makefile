BINARY := go-route-impact
MODULE := github.com/pnt-team/go-route-impact-v2
CMD := ./cmd/go-route-impact

.PHONY: build install test clean

build:
	go build -o $(BINARY) $(CMD)

build-linux:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY)-linux $(CMD)

install:
	go install $(CMD)

test:
	go test ./... -v

clean:
	rm -f $(BINARY) $(BINARY)-linux

tidy:
	go mod tidy

vet:
	go vet ./...

fmt:
	gofmt -w .
