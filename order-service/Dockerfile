FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o order-service .

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/order-service .

EXPOSE 8003

CMD ["./order-service"]