FROM golang:1.24-alpine AS build
ENV GOTOOLCHAIN=auto

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/wallet-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/wallet-migrate ./cmd/migrate

FROM alpine:3.20

WORKDIR /app

RUN addgroup -S app && adduser -S app -G app
USER app

COPY --from=build /out/wallet-server /app/wallet-server
COPY --from=build /out/wallet-migrate /app/wallet-migrate
COPY migrations /app/migrations

ENV PORT=3000
ENV MIGRATIONS_PATH=file://migrations

EXPOSE 3000

CMD ["/app/wallet-server"]
