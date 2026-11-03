FROM golang:1.26 AS tools

RUN ARCH=$(uname -m) && \
    case "$ARCH" in \
      x86_64) TAILWIND_ARCH=x64 ;; \
      aarch64) TAILWIND_ARCH=arm64 ;; \
      *) echo "Unsupported architecture: $ARCH"; exit 1 ;; \
    esac && \
    wget -O /usr/local/bin/tailwindcss "https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.3/tailwindcss-linux-${TAILWIND_ARCH}" && \
    chmod +x /usr/local/bin/tailwindcss
RUN go install github.com/go-task/task/v3/cmd/task@v3.52.0

FROM tools AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN task build

FROM alpine:3.22
WORKDIR /app
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 minivoip && mkdir data && chown minivoip data
COPY --from=build /app/bin/app ./app
# All interfaces inside the container: its Caddy reaches it over the container
# network. Set MINIVOIP_TRUSTED_PROXY to that Caddy's address, never publish 8090.
ENV PORT=8090 MINIVOIP_BIND=0.0.0.0
EXPOSE 8090
USER minivoip
CMD ["./app"]
