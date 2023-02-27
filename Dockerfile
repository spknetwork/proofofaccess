FROM golang:latest

ENV NODE=${NODE}

WORKDIR /proofofaccess

COPY go.mod go.sum ./

RUN go mod download

COPY . .

