# refapp container image. Built by goreleaser (dockers_v2 in .goreleaser.yaml),
# which places the prebuilt static binary at $TARGETPLATFORM/refapp in the build
# context — this file does not compile anything.
FROM alpine:3.22

# ca-certificates for outbound HTTPS (e.g. Postgres with SslMode verify-full),
# tzdata for TZ=.
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 1000 refapp \
    && adduser -S -D -H -u 1000 -G refapp -h /var/lib/refapp refapp \
    && mkdir -p /var/lib/refapp \
    && chown refapp:refapp /var/lib/refapp

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/refapp /usr/bin/refapp
COPY zarf/docker/config.yaml /etc/refapp/config.yaml

USER refapp
WORKDIR /var/lib/refapp
VOLUME /var/lib/refapp
EXPOSE 8080

ENTRYPOINT ["/usr/bin/refapp"]
CMD ["start", "--config", "/etc/refapp/config.yaml"]
