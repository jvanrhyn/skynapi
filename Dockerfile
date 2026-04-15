# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.26.1-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags "-X main.Version=${VERSION} -X main.CommitHash=${COMMIT} -X main.BuildTime=${BUILD_TIME} -s -w" \
    -o /bin/skynapi \
    ./cmd/api

# ── Run stage ─────────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1001 appuser

COPY --from=builder /bin/skynapi /usr/local/bin/skynapi

USER appuser

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/skynapi"]
