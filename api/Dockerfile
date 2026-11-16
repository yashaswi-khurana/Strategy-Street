# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/api ./cmd/api

# Final runner stage
FROM alpine:3.20

WORKDIR /app

COPY --from=builder /app/api /app/api

EXPOSE 8080

ENTRYPOINT ["/app/api"]
