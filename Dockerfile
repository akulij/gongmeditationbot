FROM golang:alpine
WORKDIR /build

RUN apk add build-base
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go env -w CGO_ENABLED=1
ENV GOCACHE=/root/.cache/go-build
RUN --mount=type=cache,target="/root/.cache/go-build" go build -o app ./cmd/app
WORKDIR /storage
CMD ["/build/app"]
