FROM golang:alpine AS builder

# Add source code
ADD ./ /go/src/github.com/dhax/go-base/

RUN cd /go/src/github.com/projectrekor/rekor-service && \
    go build && \
    mv ./rekor-service /usr/bin/rekor-service

# Multi-Stage production build
FROM alpine

RUN apk add --update ca-certificates

# Retrieve the binary from the previous stage
COPY --from=builder /usr/bin/rekor-service /usr/local/bin/rekor-service

# Set the binary as the entrypoint of the container
CMD ["rekor-service", "serve"]