# syntax=docker/dockerfile:1.4
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git gcc musl-dev openssh ca-certificates && update-ca-certificates
WORKDIR /app

# GOPRIVATE — если есть приватные импорты
ENV GOPRIVATE=github.com/0xAtelerix/*
ENV CGO_ENABLED=1
#ENV GOWORK=off

# Trust GitHub
RUN git config --global url."git@github.com:".insteadOf "https://github.com/"
RUN mkdir -p ~/.ssh && ssh-keyscan github.com >> ~/.ssh/known_hosts


COPY go.mod go.sum ./
RUN --mount=type=ssh go mod download

COPY . .
RUN go build -o appchain ./cmd/main.go

# Финальный образ
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/appchain .
ENTRYPOINT ["./appchain"]
