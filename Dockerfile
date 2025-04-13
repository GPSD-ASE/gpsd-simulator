FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /gpsd-simulator .

FROM alpine:3.21

WORKDIR /

COPY --from=builder /gpsd-simulator /gpsd-simulator

CMD ["/gpsd-simulator"]