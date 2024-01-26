FROM golang:1.20.13-alpine AS go
WORKDIR /app

ADD LICENSE README.md /app
ADD go.mod go.sum /app
ADD main.go console.go /app

RUN apk update && apk add --no-cache jq
RUN (jq -n 'env|to_entries[]|{(.key): (.value|tonumber? // .)}' | jq -s add) > /app/config.json
RUN go mod download
RUN go build -o qBittorrent-ClientBlocker

CMD /app/qBittorrent-ClientBlocker
