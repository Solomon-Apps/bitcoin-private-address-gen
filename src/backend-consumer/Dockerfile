FROM golang:1.21

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev make

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Check binary compatibility and dependencies
RUN chmod +x keyhunt && \
    echo "=== File information ===" && \
    file keyhunt && \
    echo "=== Dependencies ===" && \
    ldd keyhunt || true && \
    echo "=== Binary check ===" && \
    ./keyhunt -h || true

# Build the application
RUN go build -o main .

# Command to run the application
CMD ["./main"]
