# ARG ARCH="arm64"
# ARG OS="linux"
# FROM quay.io/prometheus/busybox-${OS}-${ARCH}:glibc
# LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

# ARG ARCH="arm64"
# ARG OS="linux"
# COPY .build/${OS}-${ARCH}/json_exporter /bin/json_exporter

FROM golang:1.19 as builder

# Create and change to the app directory.
WORKDIR /app

# Retrieve application dependencies.
# This allows the container build to reuse cached dependencies.
# Expecting to copy go.mod and if present go.sum.
COPY go.* ./
RUN go mod download

# Copy local code to the container image.
COPY . ./

# Build the binary.
RUN go build -v -o json_exporter

# Use the official Debian slim image for a lean production container.
# https://hub.docker.com/_/debian
# https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds
FROM debian:buster-slim
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy the binary to the production image from the builder stage.
COPY --from=builder /app/json_exporter /app/json_exporter

EXPOSE      7979
USER        nobody
ENTRYPOINT  [ "/app/json_exporter" ]
