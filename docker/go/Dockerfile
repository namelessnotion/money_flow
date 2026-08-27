# Build context is the repo root (see docker-compose.yml).
FROM golang:1.27-bookworm

RUN apt-get update -qq \
  && apt-get install -y --no-install-recommends postgresql-client \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go/go.mod go/go.sum ./
RUN go mod download

COPY go/ .

COPY docker/go/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
CMD ["go", "run", "./cmd/server"]
