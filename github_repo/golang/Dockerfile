FROM golang:1.25-alpine

WORKDIR /app

RUN apk add --no-cache gcc musl-dev sqlite-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Enable CGO for go-sqlite3
ENV CGO_ENABLED=1

CMD ["go", "run", "./cmd/api/main.go"]
