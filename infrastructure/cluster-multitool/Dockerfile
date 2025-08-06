FROM alpine:latest AS builder
# Comment to force change

# Install Python and pip
RUN apk add --no-cache \
    python3 \
    py3-pip \
    python3-dev \
    build-base

# Build Python dependencies in isolated environment
RUN python3 -m pip install --no-cache-dir --prefix=/opt/deps kubernetes PyJWT[crypto] requests s3cmd

FROM alpine:latest

# Install system packages, Python runtime, and tools
RUN apk add --no-cache \ 
    python3 \
    curl \
    wget \
    ca-certificates \
    jq \
    yq \
    vim \
    git \
    bind-tools \
    iputils \
    netcat-openbsd \
    mtr \
    bash

# Copy Python dependencies from builder
COPY --from=builder /opt/deps /usr/local

# Set Python path to include /usr/local packages
ENV PYTHONPATH="/usr/local/lib/python3.12/site-packages:$PYTHONPATH"

# Install talosctl
RUN wget -q https://github.com/siderolabs/talos/releases/download/v1.10.6/talosctl-linux-amd64 -O /usr/local/bin/talosctl \
    && chmod +x /usr/local/bin/talosctl

# Install kubectl
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" \
    && install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl \
    && rm kubectl

# Create non-root user (matching both previous containers)
RUN adduser -D -u 1000 opsuser

USER opsuser
WORKDIR /app

# Default to bash shell
CMD ["/bin/bash"]
