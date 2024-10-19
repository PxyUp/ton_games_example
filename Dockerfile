FROM golang:1.23 as build-env

WORKDIR /go/src/app

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 go build -o /go/bin/app cmd/app/main.go

FROM gcr.io/distroless/static

COPY --from=build-env /go/bin/app /
CMD ["/app"]