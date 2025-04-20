FROM mcr.microsoft.com/devcontainers/universal:2-linux

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy rest of the source code
COPY . .

# Make keyhunt binary executable and test it
RUN chmod +x keyhunt && \
    echo "=== File information ===" && \
    file keyhunt && \
    echo "=== Dependencies ===" && \
    ldd keyhunt || true && \
    echo "=== Binary check ===" && \
    ./keyhunt -h || true

# Build Go app
RUN go build -o main .

# Run the Go app
CMD ["./main"]
