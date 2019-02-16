FROM golang:1 as builder
WORKDIR /app
COPY app .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o doppler-injector

FROM alpine
RUN apk add --no-cache ca-certificates
COPY --from=0 /app/doppler-injector .
CMD ["./doppler-injector", "-alsologtostderr", "-v=4", "2>&1"]  

