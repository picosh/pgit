FROM golang:1.20 as builder
COPY . /app
WORKDIR /app
RUN go build -o pgit ./main.go

RUN git clone https://github.com/picosh/pico.git
RUN git clone https://github.com/picosh/ops.git

RUN ./pgit

FROM nginx:latest
COPY --from=builder /app/public /usr/share/nginx/html
EXPOSE 80
