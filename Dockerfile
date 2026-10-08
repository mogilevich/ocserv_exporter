FROM golang:1.23-bullseye AS builder

# bullseye is EOL: its security pool moved to archive.debian.org (deb.debian.org returns 404)
RUN sed -i 's|http://deb.debian.org/debian-security|http://archive.debian.org/debian-security|' /etc/apt/sources.list \
    && apt-get update && apt-get install -y --no-install-recommends \
    git \
    libsystemd-dev \
    gcc \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags "-X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o ocserv-exporter .

FROM debian:bullseye-slim

RUN sed -i 's|http://deb.debian.org/debian-security|http://archive.debian.org/debian-security|' /etc/apt/sources.list \
    && apt-get update && apt-get install -y --no-install-recommends \
    libsystemd0 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/ocserv-exporter /usr/local/bin/

EXPOSE 9617

ENTRYPOINT ["/usr/local/bin/ocserv-exporter"]
