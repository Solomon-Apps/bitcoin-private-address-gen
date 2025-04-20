FROM golang:1.21-alpine

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev make

# Set working directory
WORKDIR /app

# Copy keyhunt binary first and set permissions
COPY --chmod=755 keyhunt /app/keyhunt

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o main .

# Command to run the application
CMD ["./main"]
