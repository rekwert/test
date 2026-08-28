FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY packages/shared-go /src/packages/shared-go
WORKDIR /src/services/notification
COPY services/notification/go.mod services/notification/go.sum* ./
RUN go mod download
COPY services/notification .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /server ./cmd/server
RUN CGO_ENABLED=0 go build -o /worker ./cmd/worker

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /server /server
COPY --from=builder /worker /worker
EXPOSE 8004
CMD ["/server"]
