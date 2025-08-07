# --- Etapa de construcción ---
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

# Tidy the module and download any missing dependencies
RUN go mod tidy

# Compila la aplicación creando un binario estático.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/crawler ./cmd/crawler

# --- Etapa de producción ---
FROM alpine:latest

WORKDIR /app

# Copia únicamente el binario compilado desde la etapa de construcción.
COPY --from=builder /app/crawler .

# Expone el puerto en el que la aplicación se ejecutará.
EXPOSE 8080

# Comando para ejecutar la aplicación.
CMD ["./crawler"]