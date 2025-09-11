FROM golang:1.21 as builder

WORKDIR /app

COPY go.mod go.sum* ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o consumer2 ./main.go

FROM gcr.io/distroless/static-debian11

COPY --from=builder /app/consumer2 /

ENV SERVICE_NAME="consumer-2"

CMD ["/consumer2"]
