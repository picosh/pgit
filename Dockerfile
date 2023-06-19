FROM golang:1.20 as builder
COPY . /app
WORKDIR /app
RUN go build -o pgit ./main.go
RUN ./pgit

FROM nginx:latest
COPY --from=builder /app/public /usr/share/nginx/html
EXPOSE 80
