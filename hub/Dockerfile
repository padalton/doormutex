# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/otl-hub ./cmd/otl-hub \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/padalton/office-traffic-light" \
      org.opencontainers.image.description="office-traffic-light hub: rule engine, API, MQTT and Home Assistant discovery"
COPY --from=build /out/otl-hub /usr/local/bin/otl-hub
COPY --from=build --chown=nonroot:nonroot /out/data /data
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["otl-hub", "healthcheck"]
ENTRYPOINT ["otl-hub"]
CMD ["serve"]
