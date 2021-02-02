FROM alpine

RUN apk add --no-cache ca-certificates

COPY prometheus-docker-labels-discovery /prometheus-docker-labels-discovery

# Because of access to docker.sock, it's easier to run it as root...
#USER nobody

ENTRYPOINT ["/prometheus-docker-labels-discovery"]
EXPOSE 8080
