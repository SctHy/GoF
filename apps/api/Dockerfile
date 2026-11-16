# syntax=docker/dockerfile:1

FROM golang:1.27.4-alpine3.24 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gcfeed-api ./cmd/feed
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gcfeed-worker ./cmd/worker

FROM alpine:3.24.4

WORKDIR /app

RUN apk add --no-cache ca-certificates ffmpeg tzdata

COPY --from=builder /out/gcfeed-api ./gcfeed-api
COPY --from=builder /out/gcfeed-worker ./gcfeed-worker
COPY configs ./configs

RUN mkdir -p uploads

EXPOSE 8080

CMD ["./gcfeed-api"]
