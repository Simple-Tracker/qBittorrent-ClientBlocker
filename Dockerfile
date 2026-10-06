FROM --platform=${BUILDPLATFORM} golang:1.20.13-alpine AS go
WORKDIR /app

ARG BUILDOS BUILDARCH TARGETOS TARGETARCH GITHUB_REF PROGRAM_NIGHTLY
ENV GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=7

RUN PROGRAM_VERSION="$(basename ${GITHUB_REF})"; \
	if [ "${GOARCH}" == 'arm' ]; then \
		PROGRAM_VERSION="${PROGRAM_VERSION} (${TARGETOS}, ${TARGETARCH}v7)"; \
	else \
		PROGRAM_VERSION="${PROGRAM_VERSION} (${TARGETOS}, ${TARGETARCH})"; \
	fi; \
	if [ "${PROGRAM_NIGHTLY}" == 'true' ]; then \
		PROGRAM_VERSION="${PROGRAM_VERSION} (Nightly)"; \
	fi; \
	echo "export PROGRAM_VERSION='${PROGRAM_VERSION}'" > /envfile

RUN . /envfile; echo "Running on ${BUILDOS}/${BUILDARCH}, Building for ${TARGETOS}/${TARGETARCH}, Version: ${PROGRAM_VERSION}"

COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY internal/ ./internal/
RUN . /envfile; go build -ldflags "-w -X \"main.programVersion=${PROGRAM_VERSION}\"" -o qBittorrent-ClientBlocker

FROM alpine
WORKDIR /app

COPY --from=go /app/qBittorrent-ClientBlocker ./
COPY lang/ ./lang/
COPY LICENSE *.md *.txt *.json entrypoint.sh ./
RUN chmod +x ./entrypoint.sh
RUN apk update && apk add --no-cache jq socat

ENTRYPOINT ["/app/entrypoint.sh"]
