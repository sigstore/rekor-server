FROM golang:alpine AS builder

# Add source code
ADD ./ /go/src/github.com/dhax/go-base/

RUN cd /go/src/github.com/projectrekor/rekor-server && \
    go build && \
    mv ./rekor-server /usr/bin/rekor-server

# Multi-Stage production build
FROM alpine

RUN apk add --update ca-certificates

# Retrieve the binary from the previous stage
COPY --from=builder /usr/bin/rekor-server /usr/local/bin/rekor-server

# Set the binary as the entrypoint of the container
CMD ["rekor-server", "serve"]