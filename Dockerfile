FROM alpine:3.24
RUN apk --no-cache add ca-certificates
COPY rtbeat /rtbeat
WORKDIR /
EXPOSE 8081
ENTRYPOINT ["/rtbeat"]
