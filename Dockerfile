FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o game-server cmd/server/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /app/game-server .
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/web ./web
EXPOSE 8080 8081 8082
CMD ["./game-server", "-config", "configs/config-docker.yaml"]
