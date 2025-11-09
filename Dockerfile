# syntax=docker/dockerfile:1

FROM golang:1.22 AS development

WORKDIR /app

ENV PATH="/go/bin:${PATH}" \
    CGO_ENABLED=0 \
    GO111MODULE=on

RUN go install github.com/cosmtrek/air@v1.52.0 \
    && go install github.com/motemen/gore@latest

COPY . .

RUN if [ -f go.mod ]; then go mod download; fi

CMD ["air", "-c", ".air.toml"]
