# --- Build stage ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

# --- Final stage ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /server /usr/local/bin/server

EXPOSE 8080

ENTRYPOINT ["server"]
CMD ["--addr", ":8080", "--db", "/data/banner_rotation.db"]
