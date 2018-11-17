FROM alpine:3.8
RUN apk --no-cache add ca-certificates
RUN apk update && apk add rsync
COPY rtbeat /
WORKDIR /
ENTRYPOINT ["/rtbeat"]
