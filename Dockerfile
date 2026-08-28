# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.27.0
ARG ALPINE_VERSION=3.24

FROM golang:${GO_VERSION}-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS build

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

LABEL org.opencontainers.image.source="https://github.com/openclaw/gogcli"
LABEL org.opencontainers.image.description="Google services CLI for terminal automation"
LABEL org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 -h /home/gog gog

ENV HOME=/home/gog
WORKDIR /home/gog

COPY --from=build /out/gog /usr/local/bin/gog

USER gog
ENTRYPOINT ["gog"]
CMD ["--help"]
