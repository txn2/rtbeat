FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

# GoReleaser's dockers_v2 multi-platform build stages each binary under a
# ${TARGETPLATFORM} path (e.g. linux/amd64/rtbeat) in the build context, so the
# COPY must be platform-qualified.
ARG TARGETPLATFORM
RUN apk --no-cache add ca-certificates
COPY ${TARGETPLATFORM}/rtbeat /rtbeat
WORKDIR /
EXPOSE 8081
ENTRYPOINT ["/rtbeat"]
