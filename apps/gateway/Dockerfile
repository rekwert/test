FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY packages/shared-go /src/packages/shared-go
WORKDIR /src/apps/gateway
COPY apps/gateway/go.mod apps/gateway/go.sum* ./
RUN go mod download
COPY apps/gateway .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /gateway ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /gateway /gateway
EXPOSE 8080
CMD ["/gateway"]
