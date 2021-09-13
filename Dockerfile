FROM --platform=$BUILDPLATFORM golang as builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG BUILD_DATE

COPY . /src

WORKDIR /src

RUN env GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 go mod download && \
  export GIT_COMMIT=$(git rev-parse HEAD) && \
  export GIT_DIRTY=$(test -n "`git status --porcelain`" && echo "+CHANGES" || true) && \
  env GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 \
    go build -o prometheus-docker-labels-discovery \
    -ldflags "-X github.com/sqooba/go-common/version.GitCommit=${GIT_COMMIT}${GIT_DIRTY} \
              -X github.com/sqooba/go-common/version.BuildDate=${BUILD_DATE} \
              -X github.com/sqooba/go-common/version.Version=${VERSION}" \
    .

FROM --platform=$BUILDPLATFORM gcr.io/distroless/base

COPY --from=builder /src/prometheus-docker-labels-discovery /prometheus-docker-labels-discovery

# Because of access to docker.sock, it's easier to run it as root...
#USER nobody

ENTRYPOINT ["/prometheus-docker-labels-discovery"]
EXPOSE 8080

#HEALTHCHECK --interval=60s --timeout=10s --retries=1 --start-period=30s CMD ["/prometheus-docker-labels-discovery", "--health-check"]
