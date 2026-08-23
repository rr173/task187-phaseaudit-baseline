FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /app/phaseaudit ./cmd/phaseaudit

EXPOSE 8080
ENTRYPOINT ["/app/phaseaudit"]
CMD ["--smoke-test"]
