FROM alpine:3.22.1 AS builder

# Make a chance to commit and trigger the build

# Build arguments for versions
ARG KUBERNETES_VERSION
ARG TALOS_VERSION

# Install Python and pip
# renovate: datasource=repology depName=alpine_3_22/python3 versioning=loose
ENV PYTHON3_VERSION="3.12.11-r0"
# renovate: datasource=repology depName=alpine_3_22/py3-pip versioning=loose
ENV PIP_VERSION="25.1.1-r0"
# renovate: datasource=repology depName=alpine_3_22/build-base versioning=loose
ENV BUILD_BASE_VERSION="0.5-r3"

RUN apk add --no-cache \
    python3="${PYTHON3_VERSION}" \
    py3-pip="${PIP_VERSION}" \
    python3-dev="${PYTHON3_VERSION}" \
    build-base="${BUILD_BASE_VERSION}"

# Build Python dependencies in isolated environment
# renovate: datasource=pypi depName=kubernetes
ENV PYPI_KUBERNETES_VERSION="33.1.0"
# renovate: datasource=pypi depName=PyJWT
ENV PYPI_PYJWT_VERSION="2.10.1"
# renovate: datasource=pypi depName=requests
ENV PYPI_REQUESTS_VERSION="2.32.4"
# renovate: datasource=pypi depName=s3cmd
ENV PYPI_S3CMD_VERSION="2.4.0"

RUN python3 -m pip install --no-cache-dir --prefix=/opt/deps \
    kubernetes=="${PYPI_KUBERNETES_VERSION}" \
    PyJWT[crypto]=="${PYPI_PYJWT_VERSION}" \
    requests=="${PYPI_REQUESTS_VERSION}" \
    s3cmd=="${PYPI_S3CMD_VERSION}"

FROM alpine:3.22.1

# Build arguments for versions (must be redeclared for second stage)
ARG KUBERNETES_VERSION
ARG TALOS_VERSION

# Install system packages, Python runtime, and tools
# renovate: datasource=repology depName=alpine_3_22/python3 versioning=loose
ENV PYTHON3_VERSION="3.12.11-r0"
# renovate: datasource=repology depName=alpine_3_22/curl versioning=loose
ENV CURL_VERSION="8.14.1-r1"
# renovate: datasource=repology depName=alpine_3_22/wget versioning=loose
ENV WGET_VERSION="1.25.0-r1"
# renovate: datasource=repology depName=alpine_3_22/ca-certificates versioning=loose
ENV CA_CERTIFICATES_VERSION="20250619-r0"
# renovate: datasource=repology depName=alpine_3_22/jq versioning=loose
ENV JQ_VERSION="1.8.0-r0"
# renovate: datasource=repology depName=alpine_3_22/yq-go versioning=loose
ENV YQ_GO_VERSION="4.46.1-r1"
# renovate: datasource=repology depName=alpine_3_22/vim versioning=loose
ENV VIM_VERSION="9.1.1566-r0"
# renovate: datasource=repology depName=alpine_3_22/git versioning=loose
ENV GIT_VERSION="2.49.1-r0"
# renovate: datasource=repology depName=alpine_3_22/bind-tools versioning=loose
ENV BIND_TOOLS_VERSION="9.20.12-r0"
# renovate: datasource=repology depName=alpine_3_22/iputils versioning=loose
ENV IPUTILS_VERSION="20240905-r0"
# renovate: datasource=repology depName=alpine_3_22/netcat-openbsd versioning=loose
ENV NETCAT_OPENBSD_VERSION="1.229.1-r0"
# renovate: datasource=repology depName=alpine_3_22/mtr versioning=loose
ENV MTR_VERSION="0.95-r2"
# renovate: datasource=repology depName=alpine_3_22/bash versioning=loose
ENV BASH_VERSION="5.2.37-r0"

# renovate: datasource=repology depName=alpine_3_22/bash versioning=loose
RUN apk add --no-cache \ 
    python3="${PYTHON3_VERSION}" \
    curl="${CURL_VERSION}" \
    wget="${WGET_VERSION}" \
    ca-certificates="${CA_CERTIFICATES_VERSION}" \
    jq="${JQ_VERSION}" \
    yq-go="${YQ_GO_VERSION}" \
    vim="${VIM_VERSION}" \
    git="${GIT_VERSION}" \
    bind-tools="${BIND_TOOLS_VERSION}" \
    iputils="${IPUTILS_VERSION}" \
    netcat-openbsd="${NETCAT_OPENBSD_VERSION}" \
    mtr="${MTR_VERSION}" \
    bash="${BASH_VERSION}"

# Copy Python dependencies from builder
COPY --from=builder /opt/deps /usr/local

# Set Python path to include /usr/local packages
ENV PYTHONPATH="/usr/local/lib/python3.12/site-packages"

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
