FROM golang:1.26-alpine

RUN apk add --no-cache make bash git

WORKDIR /app

COPY . .

RUN go mod tidy

RUN make build

FROM busybox

COPY --from=0 /app/bin/gog /bin/

CMD ["/bin/gog"]