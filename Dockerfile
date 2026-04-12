FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /beeroklog-server ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /beeroklog-server /usr/local/bin/beeroklog-server
COPY internal/database/migrations /migrations
ENV MIGRATIONS_PATH=/migrations
EXPOSE 8080
ENTRYPOINT ["beeroklog-server"]
