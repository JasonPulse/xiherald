# The Herald is a single static binary with its templates, stylesheet, fonts
# and the Nordic kit assets embedded, so the runtime stage carries nothing but
# the binary and a CA bundle.

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Dependencies first so a source-only change reuses the cached download layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Cross-compiling from the build platform rather than emulating the target is
# what keeps the arm64 image as fast to build as the amd64 one.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags='-s -w' -o /out/xiherald .

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/xiherald /usr/local/bin/xiherald

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/xiherald"]
