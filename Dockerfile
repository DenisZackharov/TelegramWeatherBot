FROM golang:1.24.0-alpine

WORKDIR /app

COPY . .

RUN go mod tidy

RUN go build -o bot

ENV TELEGRAM_BOT_TOKEN=""
ENV DATABASE_URL=""

CMD ["/app/bot"]
