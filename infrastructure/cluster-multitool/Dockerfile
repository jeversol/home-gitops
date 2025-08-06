FROM alpine:3.22.1 AS builder

# Build arguments for versions
ARG KUBERNETES_VERSION
ARG TALOS_VERSION

# Install Python and pip
# renovate: datasource=repology depName=alpine_3_22/python3 versioning=loose
# renovate: datasource=repology depName=alpine_3_22/py3-pip versioning=loose
# renovate: datasource=repology depName=alpine_3_22/python3-dev versioning=loose
# renovate: datasource=repology depName=alpine_3_22/build-base versioning=loose
RUN apk add --no-cache \
    python3=3.12.11-r0 \
    py3-pip=25.1.1-r0 \
    python3-dev=3.12.11-r0 \
    build-base=0.5-r3

# Build Python dependencies in isolated environment
# renovate: datasource=pypi depName=kubernetes
# renovate: datasource=pypi depName=PyJWT
# renovate: datasource=pypi depName=requests
# renovate: datasource=pypi depName=s3cmd
RUN python3 -m pip install --no-cache-dir --prefix=/opt/deps \
    kubernetes==33.1.0 \
    PyJWT[crypto]==2.10.1 \
    requests==2.32.4 \
    s3cmd==2.4.0

FROM alpine:3.22.1

# Build arguments for versions (must be redeclared for second stage)
ARG KUBERNETES_VERSION
ARG TALOS_VERSION

# Install system packages, Python runtime, and tools
# renovate: datasource=repology depName=alpine_3_22/python3 versioning=loose
# renovate: datasource=repology depName=alpine_3_22/curl versioning=loose
# renovate: datasource=repology depName=alpine_3_22/wget versioning=loose
# renovate: datasource=repology depName=alpine_3_22/ca-certificates versioning=loose
# renovate: datasource=repology depName=alpine_3_22/jq versioning=loose
# renovate: datasource=repology depName=alpine_3_22/yq-go versioning=loose
# renovate: datasource=repology depName=alpine_3_22/vim versioning=loose
# renovate: datasource=repology depName=alpine_3_22/git versioning=loose
# renovate: datasource=repology depName=alpine_3_22/bind-tools versioning=loose
# renovate: datasource=repology depName=alpine_3_22/iputils versioning=loose
# renovate: datasource=repology depName=alpine_3_22/netcat-openbsd versioning=loose
# renovate: datasource=repology depName=alpine_3_22/mtr versioning=loose
# renovate: datasource=repology depName=alpine_3_22/bash versioning=loose
RUN apk add --no-cache \ 
    python3=3.12.11-r0 \
    curl=8.14.1-r1 \
    wget=1.25.0-r1 \
    ca-certificates=20250619-r0 \
    jq=1.8.0-r0 \
    yq-go=4.46.1-r0 \
    vim=9.1.1566-r0 \
    git=2.49.1-r0 \
    bind-tools=9.20.11-r0 \
    iputils=20240905-r0 \
    netcat-openbsd=1.229.1-r0 \
    mtr=0.95-r2 \
    bash=5.2.37-r0

# Copy Python dependencies from builder
COPY --from=builder /opt/deps /usr/local

# Set Python path to include /usr/local packages
ENV PYTHONPATH="/usr/local/lib/python3.12/site-packages:$PYTHONPATH"

# Install talosctl
RUN wget -q https://github.com/siderolabs/talos/releases/download/${TALOS_VERSION}/talosctl-linux-amd64 -O /usr/local/bin/talosctl \
    && chmod +x /usr/local/bin/talosctl

# Install kubectl
RUN curl -LO "https://dl.k8s.io/release/${KUBERNETES_VERSION}/bin/linux/amd64/kubectl" \
    && install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl \
    && rm kubectl

# Create a non-root user to run the container
RUN adduser -D -u 1000 opsuser

USER opsuser
WORKDIR /app

# Add OCI labels to link package to repository
LABEL org.opencontainers.image.source=https://github.com/jeversol/home-gitops
LABEL org.opencontainers.image.description="Universal cluster operations container with Python and shell tools"

CMD ["/bin/bash"]
