FROM golang:1.21

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy rest of the source code
COPY . .

# Git clone keyhunt
RUN git clone https://github.com/albertobsd/keyhunt.git

# Replace the /app/keyhunt.cpp with the keyhunt from the keyhunt folder
RUN cp -f keyhunt.cpp ./keyhunt/keyhunt.cpp

# Build key hunt
RUN cd ./keyhunt && make

# Copy the keyhunt binary to the app folder
RUN cp ./keyhunt/keyhunt /app/keyhunt

# Remove the keyhunt folder
RUN rm -rf ./keyhunt/

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
