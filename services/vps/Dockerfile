FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY packages/shared-go /src/packages/shared-go
WORKDIR /src/services/vps
COPY services/vps/go.mod services/vps/go.sum* ./
RUN go mod download
COPY services/vps .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /server ./cmd/server
RUN CGO_ENABLED=0 go build -o /worker ./cmd/worker
RUN CGO_ENABLED=0 go build -o /repair-change-ip ./cmd/repair-change-ip

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /server /server
COPY --from=builder /worker /worker
COPY --from=builder /repair-change-ip /repair-change-ip
EXPOSE 8003
CMD ["/server"]
