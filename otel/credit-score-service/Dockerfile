FROM golang:1.17-alpine as build
COPY . /src
WORKDIR /src
RUN go mod download && go build -o app

FROM alpine:latest
WORKDIR /src/
COPY --from=build /src/ .
EXPOSE 8080
RUN adduser -D application && chown -R application /src
CMD ["/src/app"]
# Application user
USER application
