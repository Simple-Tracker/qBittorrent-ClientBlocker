FROM --platform=$BUILDPLATFORM golang:1.20.13-alpine AS go
WORKDIR /app

ARG BUILDOS BUILDARCH TARGETOS TARGETARCH
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN echo "Running on $BUILDOS/$BUILDARCH, Building for $TARGETOS/$TARGETARCH."

ADD *LICENSE* *.md *.go go.mod go.sum .

RUN apk update && apk add --no-cache upx
RUN go mod download
RUN go build -ldflags '-w' -o qBittorrent-ClientBlocker
RUN upx -v -9 qBittorrent-ClientBlocker

FROM alpine
WORKDIR /app

COPY --from=go /app .
RUN apk update && apk add --no-cache jq

CMD ((jq -n 'env|to_entries[]|{(.key): (if ((.key|ascii_downcase) == "blocklist") then [] elif ((.key|ascii_downcase) != "qbusername" and ((.key|ascii_downcase) != "qbpassword")) then ((.value|tonumber?) // .value) else (.value) end)}' | jq -s add) > config.json) && ./qBittorrent-ClientBlocker
