# checkov:skip=CKV_DOCKER_2: a CLI that runs once and exits has nothing for a HEALTHCHECK to probe
# checkov:skip=CKV_DOCKER_3: GitHub requires a Docker action to run as root to reach GITHUB_WORKSPACE
# Stage 1 - Build binary and get CA certs. It runs on the build machine's own
# platform and cross-compiles for each target, so no emulation is needed.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Install CA certs (needed in final image)
RUN apk add --no-cache ca-certificates

# Copy Go files and download modules
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest and build
COPY . .

# Make sure rules and suggester are copied BEFORE build (or included in build context)
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o zanadir .

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
