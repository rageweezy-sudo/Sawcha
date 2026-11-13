# Customer-only SAWCHA Plus API. It has NO developer web routes or credentials.
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY services/plus-api/go.mod ./go.mod
COPY services/plus-api/*.go ./
RUN CGO_ENABLED=0 go test ./... && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /sawcha-plus .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /sawcha-plus /usr/local/bin/sawcha-plus
EXPOSE 8080
# SAWCHA_DATA_DIR must be mounted on a private persistent Railway volume.
CMD ["/usr/local/bin/sawcha-plus"]
