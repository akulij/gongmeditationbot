FROM golang:alpine
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o app ./cmd/app
WORKDIR /storage
CMD ["/build/app"]
