FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /mergemetrics ./cmd/mergemetrics

FROM alpine:3.20
RUN apk add --no-cache git ca-certificates
COPY --from=builder /mergemetrics /usr/local/bin/mergemetrics
ENTRYPOINT ["mergemetrics"]
CMD ["generate"]
