# syntax=docker/dockerfile:1.27.0

ARG GO_VERSION=1.27.3
ARG ALPINE_VERSION=3.24

FROM golang:${GO_VERSION}-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS build
RUN apk add --no-cache --upgrade ca-certificates=20261020-r0 ca-certificates-bundle=20261020-r0

RUN apk add --no-cache ca-certificates git tzdata

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X github.com/openclaw/gogcli/internal/cmd.version=${VERSION} -X github.com/openclaw/gogcli/internal/cmd.commit=${COMMIT} -X github.com/openclaw/gogcli/internal/cmd.date=${DATE}" \
    -o /out/gog ./cmd/gog

FROM alpine:${ALPINE_VERSION}
RUN apk add --no-cache --upgrade ca-certificates=20261020-r0 ca-certificates-bundle=20261020-r0

LABEL org.opencontainers.image.source="https://github.com/openclaw/gogcli"
LABEL org.opencontainers.image.description="Google services CLI for terminal automation"
LABEL org.opencontainers.image.licenses="MIT"

# /persist/gogcli is the documented persistent GOG_HOME (docs/install.md). A
# fresh named volume mounted there is seeded from the image, ownership
# included, so pre-creating it owned by gog keeps the non-root runtime able to
# initialise its state directories on first run (#1197).
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 -h /home/gog gog \
    && mkdir -p /persist/gogcli \
    && chown gog:gog /persist/gogcli

ENV HOME=/home/gog
WORKDIR /home/gog

COPY --from=build /out/gog /usr/local/bin/gog

USER gog
ENTRYPOINT ["gog"]
CMD ["--help"]
