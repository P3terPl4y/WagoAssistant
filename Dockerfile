# Start from official Go image for building (includes gcc for CGO/SQLite)
FROM golang:1.25-bookworm AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y build-essential

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app (CGO_ENABLED=0 because we use modernc.org/sqlite)
# Static linking so we can use a slim runtime image
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s -extldflags '-static'" -o wago ./src/main.go

# Start a new stage from a minimal debian image
FROM debian:bookworm-slim

# Add ca-certificates for HTTPS (needed for OpenAI/OpenRouter APIs) and tzdata for timezones
RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Create data directory for SQLite database
RUN mkdir -p /root/data

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/wago .

# Copy static assets and templates
COPY --from=builder /app/src/static ./src/static

# Expose port 3000 to the outside world
EXPOSE 3000

# Command to run the executable
CMD ["./wago"]
