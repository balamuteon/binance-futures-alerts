# Dockerfile

# Этап 1: Сборка
FROM golang:1.24.4 AS builder
WORKDIR /app
ARG SERVICE_NAME
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Собираем только тот сервис, который указан в SERVICE_NAME
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/${SERVICE_NAME} ./cmd/${SERVICE_NAME}

# Этап 2: Финальный образ
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
ARG SERVICE_NAME
# Копируем только скомпилированный бинарник из этапа сборки
COPY --from=builder /app/bin/${SERVICE_NAME} .
COPY ./static ./static