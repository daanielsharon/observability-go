FROM golang:1.23 as builder

WORKDIR /app

COPY go.mod go.sum* ./

RUN go mod download

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o consumer1 ./main.go

FROM gcr.io/distroless/static-debian11

COPY --from=builder /app/consumer1 /

ENV SERVICE_NAME="consumer-1"

CMD ["/consumer1"]
