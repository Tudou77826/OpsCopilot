FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY pkg ./pkg
COPY internal ./internal
COPY cmd/service-center ./cmd/service-center
RUN CGO_ENABLED=0 go build -trimpath -o /service-center ./cmd/service-center

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -g 10001 ops && adduser -D -u 10001 -G ops ops && mkdir /data && chown ops:ops /data
COPY --from=build /service-center /usr/local/bin/service-center
USER ops
ENV OPS_SERVICE_DATA=/data OPS_SERVICE_LISTEN=0.0.0.0:9080
EXPOSE 9080
ENTRYPOINT ["/usr/local/bin/service-center"]
