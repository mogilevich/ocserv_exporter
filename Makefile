VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY = ocserv-exporter
LDFLAGS = -ldflags "-X main.version=$(VERSION)"

.PHONY: all build test clean install docker fmt lint

all: build

build:
	go build $(LDFLAGS) -o $(BINARY) .

build-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-linux-amd64 .

test:
	go test -v ./...

clean:
	rm -f $(BINARY) $(BINARY)-linux-amd64

install: build-linux
	scp $(BINARY)-linux-amd64 root@vpn1.example.com:/usr/local/bin/$(BINARY)
	scp systemd/ocserv-exporter.service root@vpn1.example.com:/etc/systemd/system/
	ssh root@vpn1.example.com "systemctl daemon-reload && systemctl enable --now ocserv-exporter"

docker:
	docker build -t ocserv-exporter:$(VERSION) .

fmt:
	go fmt ./...

lint:
	golangci-lint run
