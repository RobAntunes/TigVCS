# Development environment for Tig
FROM golang:1.23.2-alpine

# Add essential build tools
RUN apk add --no-cache \
    git \
    make \
    gcc \
    musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the code
COPY . .

# Expose default port
EXPOSE 8080

# Run development server with hot reload
CMD ["go", "run", "main.go"]