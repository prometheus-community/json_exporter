FROM golang:1.11 as builder
WORKDIR /go/src/github.com/coveo/prometheus-json-exporter/
ADD  . .
RUN ./gow get . && ./gow build -o json-exporter


FROM alpine:3.8
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /go/src/github.com/coveo/prometheus-json-exporter/json-exporter .
# Create user
ARG uid=1000
ARG gid=1000
ARG username="json-exporter"
RUN addgroup -g $gid $username
RUN adduser -D -u $uid -G $username $username
RUN chown -R $username:$username /app

# Run container as $username
USER $username

ENTRYPOINT ["./json-exporter"]