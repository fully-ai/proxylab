FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/proxylab ./cmd/proxylab

FROM alpine:3.21
RUN adduser -D -H appuser
COPY --from=builder /out/proxylab /usr/local/bin/proxylab
USER appuser
EXPOSE 8085
ENTRYPOINT ["proxylab"]
CMD ["serve", "-addr", "0.0.0.0:8085"]
