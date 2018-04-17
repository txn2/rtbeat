FROM alpine:3.7
RUN apk --no-cache add ca-certificates
RUN apk update && apk add rsync
COPY rtbeat /
WORKDIR /
ENTRYPOINT ["/rtbeat"]
