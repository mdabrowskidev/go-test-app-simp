FROM golang:1.27-alpine

WORKDIR /app

COPY . .

RUN go get -d -v ./...

RUN go build -o api .

RUN adduser -D -u 1000 appuser
USER 1000

EXPOSE 8080

CMD ["./api"]

#   docker buildx build --platform linux/amd64 -t artifactory.wikia-inc.com/mdabrowski/go-test-app:<REVISION NUMBER>--push .
