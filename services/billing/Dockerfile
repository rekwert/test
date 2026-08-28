FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY packages/shared-go /src/packages/shared-go
WORKDIR /src/services/billing
COPY services/billing/go.mod services/billing/go.sum* ./
RUN go mod download
COPY services/billing .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /server ./cmd/server
RUN CGO_ENABLED=0 go build -o /worker ./cmd/worker

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY services/billing/certs/russian_trusted_root_ca.pem /usr/local/share/ca-certificates/russian_trusted_root_ca.crt
COPY services/billing/certs/russian_trusted_sub_ca.pem /usr/local/share/ca-certificates/russian_trusted_sub_ca.crt
RUN update-ca-certificates
COPY --from=builder /server /server
COPY --from=builder /worker /worker
EXPOSE 8002
CMD ["/server"]
