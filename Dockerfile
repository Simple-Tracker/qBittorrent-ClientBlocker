FROM --platform=$BUILDPLATFORM golang:1.20.13-alpine AS go

WORKDIR /app

ARG BUILDOS BUILDARCH TARGETOS TARGETARCH
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN echo "Running on $BUILDOS/$BUILDARCH, Building for $TARGETOS/$TARGETARCH."

ADD LICENSE README.md /app
ADD go.mod go.sum /app
ADD main.go console.go /app

RUN apk update && apk add --no-cache jq
RUN go mod download
RUN go build -o qBittorrent-ClientBlocker

FROM alpine
COPY --from=go /app /app
CMD ((jq -n 'env|to_entries[]|{(.key): (.value|tonumber? // .|(if . == "true" then true elif . == "false" then false else . end))}' | jq -s add) > /app/config.json) && /app/qBittorrent-ClientBlocker
