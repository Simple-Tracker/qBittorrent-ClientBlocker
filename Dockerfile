FROM --platform=$BUILDPLATFORM golang:1.20.13-alpine AS go
WORKDIR /app

ARG BUILDOS BUILDARCH TARGETOS TARGETARCH
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN echo "Running on $BUILDOS/$BUILDARCH, Building for $TARGETOS/$TARGETARCH."

ADD *LICENSE* config.json *.md *.go *.sh go.mod go.sum ./

RUN apk update && apk add --no-cache upx
RUN go mod download
RUN go build -ldflags '-w' -o qBittorrent-ClientBlocker
RUN upx -v -9 qBittorrent-ClientBlocker

FROM alpine
WORKDIR /app

COPY --from=go /app ./
RUN chmod +x ./entrypoint.sh
RUN apk update && apk add --no-cache jq

ENTRYPOINT ["./entrypoint.sh"]
