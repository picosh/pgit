FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . /app

RUN go build -v -o pgit main.go

FROM debian:12
WORKDIR /app

RUN apt-get update && apt-get install -y git
# ignore git warning "detected dubious ownership in repository"
RUN git config --global safe.directory '*'

COPY --from=builder /app/pgit /usr/bin

CMD ["pgit"]
