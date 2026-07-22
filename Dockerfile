# syntax=docker/dockerfile:1.7

FROM node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS go-build
ARG VERSION=dev
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/web-terminal ./

FROM alpine:3.22
RUN apk add --no-cache ca-certificates docker-cli tzdata util-linux \
    && addgroup -g 10001 -S webterminal \
    && adduser -u 10001 -S -D -H -G webterminal webterminal \
    && mkdir -p /run/web-terminal \
    && chown 10001:10001 /run/web-terminal
COPY --from=go-build /out/web-terminal /usr/local/bin/web-terminal
EXPOSE 3000
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/web-terminal"]
CMD ["server"]
