FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM build AS test
RUN go test ./...
RUN go vet ./...

FROM build AS binary
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates
RUN addgroup -S gateway && adduser -S -G gateway gateway
COPY --from=binary /out/gateway /usr/local/bin/gateway
USER gateway
EXPOSE 8443
ENTRYPOINT ["/usr/local/bin/gateway"]
