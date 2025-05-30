# Stage 1: Build the Go application
FROM golang:1.21-alpine AS builder

ARG Version
ARG Commit
ARG Date

# Set necessary environment variables for building
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

# Create a non-root user and group in the builder stage
# This is good practice, though the final stage is more critical for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /build

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

# Copy the rest of the application source code
COPY . .

# Build the application
# The -ldflags part is to inject version information if your main.go supports it.
# Based on your main.go, it seems like Version, Commit, and Date are variables.
# We'll use a placeholder for now. You might want to inject actual git info in CI.
RUN go build -ldflags="-X main.Version=${Version} -X main.Commit=${Commit} -X main.Date=${Date}" -o discord2pushover .

# Stage 2: Create the final lightweight image
FROM alpine:latest

# Create a non-root user and group
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Create a directory for the application
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /build/discord2pushover /app/discord2pushover

# Copy the default configuration file name(s) if they exist in the repo root,
# so users can run the container without always mounting if a default config is suitable.
# However, the README states config is looked for in CWD or via -c.
# For a container, it's better to establish a clear config path, e.g., /app/config.
# Let's assume the config will be mounted to /app/discord2pushover.yaml or /app/discord2pushover.yml
# No COPY for config here, as it should be mounted by the user.

# Set permissions for the appuser
# The binary needs to be executable by the appuser
RUN chown appuser:appgroup /app/discord2pushover &&     chmod u+x /app/discord2pushover

# Switch to the non-root user
USER appuser

# Set the default command to run the application.
# Users can override this or add -c /path/to/config.yaml
# We'll point it to look for config in /app by default if no -c is given by user.
# The application already looks for discord2pushover.yaml/yml in CWD.
# Since WORKDIR is /app, this should work if config is mounted to /app.
ENTRYPOINT ["/app/discord2pushover"]

# Default arguments can be provided in CMD
# If the user provides arguments to `docker run`, these CMD args are overridden.
# Example: CMD ["-c", "/app/config/discord2pushover.yaml"]
# For now, let's assume the app's default behavior of checking CWD (/app) is sufficient.
CMD []
