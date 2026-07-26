# Multi-stage build producing a single static binary on a distroless base.
FROM golang:1.26 AS build
WORKDIR /src

# Cache module downloads separately from source.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
# CGO is off (modernc sqlite is pure Go), so the binary is fully static.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/spoilarr ./cmd/spoilarr

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/spoilarr /app/spoilarr

ENV SPOILARR_DATA_DIR=/data \
    SPOILARR_ADDR=:8484
VOLUME ["/data"]
EXPOSE 8484

ENTRYPOINT ["/app/spoilarr"]
