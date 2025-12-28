ARG GO_VERSION=1.25.4

FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download && go mod verify && go mod tidy

RUN GOFLAGS='-buildvcs=false' go build -o bot ./cmd/
RUN chmod +x .

COPY . .

ENV GOPATH=/go

RUN go install github.com/x-motemen/gore/cmd/gore@latest && \
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.2

RUN apk add --no-cache ca-certificates tzdata && update-ca-certificates

RUN addgroup -g 1000 appuser && adduser -D -u 1000 -G appuser appuser

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER appuser

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

EXPOSE 8080

CMD ["./bot"]