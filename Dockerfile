FROM golang:1.11

WORKDIR /go/src/FlamingoV2
COPY . .
RUN go get github.com/tools/godep
RUN godep restore
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest
WORKDIR /root/
COPY --from=0 /go/src/FlamingoV2/app .
CMD ["./app"]
