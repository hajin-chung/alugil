FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/alugil ./cmd/alugil

FROM alpine:3.20
RUN adduser -D -H -u 10001 appuser
WORKDIR /app

COPY --from=build /out/alugil /usr/local/bin/alugil
COPY config.example.yaml /app/config.example.yaml

USER appuser
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/alugil"]
