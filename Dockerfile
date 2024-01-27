FROM --platform=$BUILDPLATFORM golang:1.20.13-alpine AS go
WORKDIR /app

ARG BUILDOS BUILDARCH TARGETOS TARGETARCH
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN echo "Running on $BUILDOS/$BUILDARCH, Building for $TARGETOS/$TARGETARCH."

ADD LICENSE README.md .
ADD go.mod go.sum .
ADD main.go console.go .

RUN go mod download
RUN go build -o qBittorrent-ClientBlocker

FROM alpine
WORKDIR /app

COPY --from=go /app .
RUN apk update && apk add --no-cache jq

CMD ((jq -n 'env|to_entries[]|{(.key): (.value|tonumber? // .|(if . == "true" then true elif . == "false" then false else . end))}' | jq -s add) > config.json) && ./qBittorrent-ClientBlocker
