FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

# Собираем Go-приложение.
# - CGO_ENABLED=0 отключает Cgo, что позволяет создавать статические бинарники (важно для маленьких образов типа Alpine/Scratch)
# - GOOS=linux собирает бинарник для Linux (так как финальный образ будет Alpine Linux)
# -o /app/server указывает, что скомпилированный бинарник будет называться 'server' и находиться в /app/
# Путь к вашему main пакету - ./cmd/ (если main.go внутри этой папки)
RUN CGO_ENABLED=0 GOOS=linux go build -v -o /app/server ./cmd/

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/static ./static/

COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]