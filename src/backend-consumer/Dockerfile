FROM golang:1.21-alpine

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev make

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code and keyhunt
COPY . .
RUN chmod +x keyhunt

# Build the application
RUN go build -o main .

# Command to run the application
CMD ["./main"]
