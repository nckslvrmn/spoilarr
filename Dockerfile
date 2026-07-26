FROM public.ecr.aws/docker/library/golang:1.26-alpine3.23 AS go-builder
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
# modernc SQLite is pure Go, so the binary builds fully static with CGO off.
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o spoilarr ./cmd/spoilarr
# Assemble a minimal rootfs: CA certs (for plex.tv), a non-root user, and a
# /data directory owned by that user so a mounted volume inherits write access.
RUN mkdir -p /runtime/etc/ssl/certs /runtime/data /runtime/tmp && \
    cp /etc/ssl/certs/ca-certificates.crt /runtime/etc/ssl/certs/ca-certificates.crt && \
    printf 'spoilarr:x:911:911:Spoilarr:/home/spoilarr:/sbin/nologin\n' > /runtime/etc/passwd && \
    printf 'spoilarr:x:911:\n' > /runtime/etc/group && \
    chown -R 911:911 /runtime/data && \
    chmod 1777 /runtime/tmp

FROM scratch
ARG VERSION=dev
ARG REVISION=unknown
ARG SOURCE=https://github.com/nckslvrmn/spoilarr
LABEL org.opencontainers.image.title="spoilarr" \
      org.opencontainers.image.description="Self-hosted Plex spoiler protection" \
      org.opencontainers.image.source="$SOURCE" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$REVISION"
ENV SPOILARR_DATA_DIR=/data \
    SPOILARR_ADDR=:8484 \
    TMPDIR=/tmp
COPY --from=go-builder /runtime/ /
COPY --from=go-builder --chmod=0755 /src/spoilarr /spoilarr
USER 911:911
EXPOSE 8484
VOLUME ["/data"]
ENTRYPOINT ["/spoilarr"]
