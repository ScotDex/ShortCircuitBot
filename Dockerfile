# Dockerfile

# --- Stage 1: The Builder ---
# This stage uses the official Go image to build your application.
FROM golang:1.25-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy module files and download dependencies first. This step is cached by Docker,
# which makes future builds much faster if your dependencies haven't changed.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of your application's source code
COPY . .

# Build the Go application into a single, static binary.
# CGO_ENABLED=0 ensures it has no dependencies on C libraries.
# -ldflags="-w -s" strips debug info, making the final file smaller.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /shortcircuit-bot .


# --- Stage 2: The Final Image ---
# This stage starts from a tiny Alpine Linux image to create the final,
# lightweight container for running the bot.
FROM alpine:latest

# Copy only the compiled binary from the 'builder' stage.
# None of the source code or build tools are included in the final image.
COPY --from=builder /shortcircuit-bot /shortcircuit-bot


COPY mapSolarSystemJumps.csv .
COPY system_cache.json .
COPY tripwire_data.json .
COPY system_kills.json .

# Set the command that will be executed when the container starts.
ENTRYPOINT ["/shortcircuit-bot"]