# Build stage
FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# Bake build information into the served OpenAPI spec. Placeholder tokens are
# replaced so the image's /assets/openapi/openapi.yaml reflects the real build.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN sed -i -e "s|__VERSION__|${VERSION}|g" -e "s|__BUILD_DATE__|${BUILD_DATE}|g" assets/openapi/openapi.yaml

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildDate=${BUILD_DATE}" -o bayeos-api .

# Run stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/bayeos-api .
COPY migrations/ /migrations/
COPY --from=builder /app/assets/ /assets/

EXPOSE 5532

ENTRYPOINT ["./bayeos-api"]
