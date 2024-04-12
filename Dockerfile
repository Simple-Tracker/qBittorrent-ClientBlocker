FROM --platform=${BUILDPLATFORM} golang:1.20.13-alpine AS go
WORKDIR /app

ARG BUILDOS BUILDARCH TARGETOS TARGETARCH PROGRAM_NIGHTLY
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
	export PROGRAM_VERSION

RUN echo "Running on ${BUILDOS}/${BUILDARCH}, Building for ${TARGETOS}/${TARGETARCH}, Version: ${PROGRAM_VERSION}"

ADD lang/ *LICENSE* *.md *.go *.sh go.mod go.sum config.json ./

RUN go mod download
RUN go build -ldflags "-w -X \"main.programVersion=${PROGRAM_VERSION}\"" -o qBittorrent-ClientBlocker
RUN rm -f *.go go.mod go.sum

FROM alpine
WORKDIR /app

COPY --from=go /app ./
RUN chmod +x ./entrypoint.sh
RUN apk update && apk add --no-cache jq

ENTRYPOINT ["./entrypoint.sh"]
