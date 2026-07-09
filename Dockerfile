FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY static static
RUN CGO_ENABLED=0 go build -o /xbattle-web .

FROM scratch
COPY --from=build /xbattle-web /xbattle-web
EXPOSE 8080
ENTRYPOINT ["/xbattle-web"]
