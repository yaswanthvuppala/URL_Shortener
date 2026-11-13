# Build Stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and assets
COPY . .

# Build a statically linked, CGO-free binary for Linux
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/server ./cmd/server

# Final Stage: Minimal, secure runtime
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy executable
COPY --from=builder /app/bin/server /app/bin/server

# Directory for SQLite data persistence if used
RUN mkdir -p /app/data

# Default environment configuration
ENV PORT=8000 \
    BASE_URL=http://localhost:8000 \
    DATABASE_URL=sqlite:///./data/url_shortener.db \
    REDIS_URL=redis://localhost:6379

EXPOSE 8000

CMD ["/app/bin/server"]
