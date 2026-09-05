# Renovate webhook receiver

This service receives the repository's GitHub webhook events at
`POST /hooks/renovate-dependency-dashboard`. It replaces `webhookd`, which
passed JSON request bodies as a process argument and failed when Renovate's
Dependency Dashboard payload exceeded Linux's per-argument size limit.

The receiver:

- reads request bodies directly from HTTP, with a configurable 4 MiB limit;
- verifies `X-Hub-Signature-256` against the raw body using `GITHUB_SECRET`;
- detects dashboard and Renovate PR checkbox transitions;
- creates a Job from the in-cluster Renovate CronJob;
- preserves Pushover and Discord notifications for assigned Renovate PRs; and
- serves `GET /healthz` for Kubernetes probes.

It uses only the Go standard library and runs as a non-root static binary in a
distroless image. Kubernetes access is authenticated with the mounted service
account token and cluster CA.

Run the tests with:

```sh
go test ./...
```
