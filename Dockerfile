ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:glibc
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/json_exporter /bin/json_exporter

EXPOSE      7979
USER        nobody
ENTRYPOINT  [ "/bin/json_exporter" ]
