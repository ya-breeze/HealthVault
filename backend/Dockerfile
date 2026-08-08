FROM golang:1.26 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# sqlite_fts5 enables SQLite's FTS5 module in mattn/go-sqlite3; the USDA food
# index is an FTS5 virtual table and fails at runtime without it.
RUN CGO_ENABLED=1 GOOS=linux go build -tags sqlite_fts5 -o /hcw ./cmd/

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates sqlite3 && rm -rf /var/lib/apt/lists/*
COPY --from=builder /hcw /hcw
EXPOSE 8080
CMD ["/hcw", "server"]
