FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY . .
RUN go mod tidy && go build -o /twitter ./cmd/api

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /twitter .
EXPOSE 8080
CMD ["./twitter"]
