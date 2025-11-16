FROM alpine:3.22.2 AS builder

RUN apk add --no-cache \
    python3 \
    py3-pip \
    python3-dev \
    build-base \
    libffi-dev

# renovate: datasource=pypi depName=kubernetes
ENV PYPI_KUBERNETES_VERSION="34.1.0"
# renovate: datasource=pypi depName=PyJWT
ENV PYPI_PYJWT_VERSION="2.10.1"
# renovate: datasource=pypi depName=requests
ENV PYPI_REQUESTS_VERSION="2.32.5"
# renovate: datasource=pypi depName=s3cmd
ENV PYPI_S3CMD_VERSION="2.4.0"

RUN python3 -m pip install --no-cache-dir --prefix=/opt/deps \
    kubernetes=="${PYPI_KUBERNETES_VERSION}" \
    PyJWT[crypto]=="${PYPI_PYJWT_VERSION}" \
    requests=="${PYPI_REQUESTS_VERSION}" \
    s3cmd=="${PYPI_S3CMD_VERSION}"

FROM alpine:3.22.2

# Build environment variables for versions
# renovate: datasource=github-tags depName=kubernetes/kubernetes versioning=semver
ENV KUBERNETES_VERSION="v1.34.2"
# renovate: datasource=github-tags depName=siderolabs/talos versioning=semver
ENV TALOS_VERSION="v1.11.5"

RUN apk add --upgrade --no-cache \
    python3 \
    curl \
    wget \
    ca-certificates \
    jq \
    yq-go \
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
ENV PYTHONPATH=/usr/local/lib/python3.12/site-packages

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
