FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY packages/shared-go /src/packages/shared-go
WORKDIR /src/services/telegram-bot
COPY services/telegram-bot/go.mod ./
RUN go mod download
COPY services/telegram-bot/ .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /bot ./cmd/bot

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /bot /bot
CMD ["/bot"]
