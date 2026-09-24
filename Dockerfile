FROM golang:1.27-alpine

WORKDIR /app

COPY . .

RUN go get -d -v ./...

RUN go build -o api .

EXPOSE 8000

CMD ["./api"]