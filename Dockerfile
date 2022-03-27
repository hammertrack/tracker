FROM golang:1.18-alpine AS build

WORKDIR /src

# Caching order:
# Deps
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Source code
COPY . .

# Build
# Don't use libc. The resulting binary will be statically linked against the
# libraries so no C libraries will be called
ENV CGO_ENABLED=0
RUN go build -o /usr/local/bin/app .

ENTRYPOINT ["app"]
