# syntax=docker/dockerfile:1.7

# Build from the repository root so the local replace of libs/go/geo resolves:
#   docker build -f services/api/Dockerfile .

FROM golang:1.26.3-bookworm AS build
WORKDIR /src

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOWORK=off

COPY libs/go/geo ./libs/go/geo
COPY services/api/go.mod services/api/go.sum ./services/api/
RUN cd services/api && go mod download

COPY services/api ./services/api

ARG VERSION=dev
RUN cd services/api && \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
