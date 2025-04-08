# Stage 1 - Build binary and get CA certs
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install CA certs (needed in final image)
RUN apk add --no-cache ca-certificates

# Copy Go files and download modules
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest and build
COPY . .

# Make sure rules and suggester are copied BEFORE build (or included in build context)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o zanadir .

# Stage 2 - Create tiny final image
FROM scratch

# Copy binary and CA certs
COPY --from=builder /app/zanadir /zanadir
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/rules /rules
COPY --from=builder /app/suggester /suggester

# Set working directory
WORKDIR /

# Define entrypoint
ENTRYPOINT ["/zanadir"]
