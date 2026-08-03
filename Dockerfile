FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO off, as the release workflow already builds. Nothing here needs a C
# toolchain — the SQLite driver is modernc's pure-Go one — and this image
# installs no compiler, so asking for cgo only risks a build that cannot link.
RUN CGO_ENABLED=0 go build -o /mu .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /mu /usr/local/bin/mu
RUN mkdir -p /data
# HOME, not DATA_DIR. Everything Mu writes goes under $HOME/.mu — data,
# settings, keys — so HOME is the one knob that puts state on the volume.
# DATA_DIR used to be set here and was read by nothing: the volume mounted at
# /data stayed empty while the real data sat in the container's own filesystem
# and went with it on the next `docker compose up --build`.
ENV HOME=/data
EXPOSE 8080 8081 2525
VOLUME /data
ENTRYPOINT ["mu", "--serve"]
