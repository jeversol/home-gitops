FROM debian:bookworm-slim

# Install system packages and tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    # Core utilities
    curl \
    wget \
    ca-certificates \
    jq \
    yq \
    vim \
    git \
    # Network debugging
    dnsutils \
    iputils-ping \
    netcat-openbsd \
    mtr-tiny \
    # S3 tools
    s3cmd \
    # Clean up
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install talosctl
RUN wget -q https://github.com/siderolabs/talos/releases/download/v1.10.6/talosctl-linux-amd64 -O /usr/local/bin/talosctl \
    && chmod +x /usr/local/bin/talosctl

# Install kubectl
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" \
    && install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl \
    && rm kubectl

# Create non-root user
RUN adduser --disabled-password --gecos '' --uid 1000 opsuser

USER opsuser
WORKDIR /app

# Default to bash shell
CMD ["/bin/bash"]