FROM golang:1.11

WORKDIR /go/src/app
COPY . .
RUN go get github.com/tools/godep
RUN godep restore
RUN CGO_ENABLED=) GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest
WORKDIR /root/
COPY --from=0 /go/src/FlamingoV2/app .
RUN apk update \
    && apk add ca-certificates \
    && update-ca-certificates \
    && apk add openssl
CMD ["./app"]