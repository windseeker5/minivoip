# minivoip in a container: the real one, behind Caddy at voip.dresdell.com.
# It runs from ~/Documents/DEV/docker-compose.yml (service "minivoip"), with
# this folder's data/ mounted at /app/data. See docs/architecture.md "Running for real".

# Build: CSS, the component bundle and templ's Go code are made fresh here, so the
# image never carries the dev watcher's files.
FROM golang:1.27-alpine AS build
ARG TAILWIND=v4.3.3
RUN apk add --no-cache curl libstdc++ libgcc \
 && curl -fsSL -o /usr/local/bin/tailwindcss \
      https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND}/tailwindcss-linux-x64-musl \
 && chmod +x /usr/local/bin/tailwindcss
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN tailwindcss -i ./assets/css/globals.css -o ./assets/css/output.css --minify \
 && go tool shadcn-templ bundle \
 && go tool templ generate \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/minivoip .

# Run: one binary, plus ffmpeg, which splits a call's two voices for notes.
FROM alpine:3.22
RUN apk add --no-cache ffmpeg ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/minivoip /usr/local/bin/minivoip
USER 1000:1000
CMD ["minivoip"]
