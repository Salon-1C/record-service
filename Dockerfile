FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /record-service ./cmd/api

FROM alpine:3.20

WORKDIR /app

RUN adduser -D -u 10001 appuser

COPY --from=builder /record-service /usr/local/bin/record-service

USER appuser

EXPOSE 8081

ENTRYPOINT ["/usr/local/bin/record-service"]
