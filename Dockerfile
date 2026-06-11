FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /mu .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /mu /usr/local/bin/mu
RUN mkdir -p /data
ENV DATA_DIR=/data
EXPOSE 8080 8081 2525
VOLUME /data
ENTRYPOINT ["mu", "--serve"]
