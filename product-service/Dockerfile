FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o product-service .

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/product-service .

EXPOSE 8001

CMD ["./product-service"]