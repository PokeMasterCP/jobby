FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY db/migrations ./db/migrations
COPY internal ./internal
COPY web ./web

RUN go build -o /jobby .

FROM alpine:3.22

COPY --from=build /jobby /usr/local/bin/jobby

EXPOSE 8080

CMD ["jobby"]
