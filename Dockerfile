# Build stage
FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.Version=${VERSION}" -o bayeos-api .

# Run stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/bayeos-api .
COPY migrations/ /migrations/
COPY assets/ /assets/

EXPOSE 5532

ENTRYPOINT ["./bayeos-api"]
