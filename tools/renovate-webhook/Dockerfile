FROM golang:1.26.4-alpine3.22 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /renovate-webhook .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /renovate-webhook /renovate-webhook
LABEL org.opencontainers.image.source="https://github.com/jeversol/home-gitops"
LABEL org.opencontainers.image.description="GitHub webhook receiver for triggering Renovate jobs"
USER 65532:65532
ENTRYPOINT ["/renovate-webhook"]
