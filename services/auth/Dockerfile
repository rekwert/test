FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY packages/shared-go /src/packages/shared-go
WORKDIR /src/services/auth
COPY services/auth/go.mod services/auth/go.sum* ./
RUN go mod download
COPY services/auth .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /server /server
EXPOSE 8001
CMD ["/server"]
