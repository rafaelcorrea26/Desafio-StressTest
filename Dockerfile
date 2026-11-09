FROM golang:1.22-alpine AS builder
WORKDIR /app

# baixa as dependências antes de copiar o resto pra aproveitar o cache do docker
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o stresstest .

FROM scratch
# sem os certificados a imagem scratch não consegue chamar nada em https
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/stresstest /stresstest
ENTRYPOINT ["/stresstest"]
