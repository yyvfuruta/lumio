FROM --platform=$BUILDPLATFORM golang:1.26.11-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=none
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION -X main.commit=$COMMIT" -o /out/lumio ./cmd/lumio
RUN mkdir -p /out/data && chown 65532:65532 /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lumio /lumio
COPY --from=build --chown=65532:65532 /out/data /data
ENV LUMIO_ADDR=:8080 LUMIO_DATA_DIR=/data
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/lumio"]
