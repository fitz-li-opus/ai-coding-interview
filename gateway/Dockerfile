FROM golang:1.23-alpine

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata curl wget

RUN go install github.com/air-verse/air@v1.61.1

EXPOSE 8080

CMD ["air", "-c", ".air.toml"]
