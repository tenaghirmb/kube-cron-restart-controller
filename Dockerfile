# Build the manager binary
FROM golang:1.26 as builder

LABEL maintainer="tenag_hirmb@hotmail.com"

# Copy in the go src
WORKDIR /go/src/github.com/tenaghirmb/cron-restart
COPY . /go/src/github.com/tenaghirmb/cron-restart/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/main.go

# Copy the controller-manager into a thin image
FROM alpine:3.24
RUN apk add --no-cache tzdata

RUN addgroup -g 10001 -S appgroup && \
    adduser -u 10001 -S appuser -G appgroup

WORKDIR /app

COPY --from=builder --chown=appuser:appgroup /go/src/github.com/tenaghirmb/cron-restart/manager .

USER 10001

CMD ["/app/manager"]
