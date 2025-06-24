# # Dockerfile

# # Этап 1: Сборка
# FROM golang:1.24.4 AS builder
# WORKDIR /app
# ARG SERVICE_NAME
# COPY go.mod go.sum ./
# RUN go mod download
# COPY . .
# # Собираем только тот сервис, который указан в SERVICE_NAME
# RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/${SERVICE_NAME} ./cmd/${SERVICE_NAME}

# # Этап 2: Финальный образ
# FROM alpine:latest
# RUN apk --no-cache add ca-certificates
# WORKDIR /app
# ARG SERVICE_NAME
# # Копируем только скомпилированный бинарник из этапа сборки
# COPY --from=builder /app/bin/${SERVICE_NAME} .
# COPY ./static ./static

# Dockerfile

# --- Этап 1: Сборка приложения ---
FROM golang:1.24.4-alpine AS builder

# Указываем, какой сервис мы собираем.
# Это значение будет передаваться из docker-compose или GitHub Actions.
ARG SERVICE_NAME

WORKDIR /app

# Копируем файлы зависимостей и загружаем их отдельно для кеширования
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь остальной исходный код
COPY . .

# Собираем бинарный файл для конкретного сервиса
# CGO_ENABLED=0 и GOOS=linux нужны для статической сборки под Alpine
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/${SERVICE_NAME} ./cmd/${SERVICE_NAME}


# --- Этап 2: Финальный легковесный образ ---
FROM alpine:latest

# Устанавливаем корневые сертификаты для HTTPS/WSS соединений
RUN apk --no-cache add ca-certificates

# Указываем рабочий каталог
WORKDIR /app

# Копируем только скомпилированный бинарный файл из этапа сборки
COPY --from=builder /app/bin/* /app/

# Копируем статические файлы, если они нужны (например, для gateway)
COPY static ./static

# Указываем, что сервис будет работать на порту 8080 (для gateway)
EXPOSE 8080

# Команда по умолчанию будет просто запуск бинарного файла,
# но мы будем переопределять ее в docker-compose и GitHub Actions.