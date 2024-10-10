FROM golang:1.23.1-alpine AS builder
WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY cmd/ ./cmd/
COPY acme/ ./acme/
COPY bgrun/ ./bgrun/
COPY cron/ ./cron/
COPY db/ ./db/
COPY handlers/ ./handlers/
COPY i18n/ ./i18n/
COPY logscan/ ./logscan/
COPY metrics/ ./metrics/
COPY pack/ ./pack/
COPY public/ ./public/
COPY tpl/ ./tpl/
COPY widgets/ ./widgets/

COPY *.go ./

RUN GOOS=linux CGO_ENABLED=1 go build -o goatcounter -ldflags="-X zgo.at/goatcounter/v2.Version=Docker" ./cmd/goatcounter

FROM scratch
WORKDIR /app

COPY --from=builder /app /app/
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

CMD [ "/app/goatcounter serve -db sqlite+$db_path" ]
