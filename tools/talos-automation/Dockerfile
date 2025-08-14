FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o talos-automation .

FROM alpine:latest

# Install talosctl and other dependencies
RUN apk add --no-cache ca-certificates curl git openssl jq && \
    TALOS_VERSION=$(curl -s https://api.github.com/repos/siderolabs/talos/releases/latest | jq -r .tag_name) && \
    ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/') && \
    curl -Lo /usr/local/bin/talosctl "https://github.com/siderolabs/talos/releases/download/${TALOS_VERSION}/talosctl-linux-${ARCH}" && \
    chmod +x /usr/local/bin/talosctl

WORKDIR /app

COPY --from=builder /app/talos-automation .

EXPOSE 3847

CMD ["./talos-automation"]