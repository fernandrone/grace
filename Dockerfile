FROM golang:1.14.7
ENV CGO_ENABLED=0 GOARCH=amd64 GOOS=linux
WORKDIR /go/src/github.com/fernandrone/grace/
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags="-w -s" -o /bin/grace

FROM scratch
COPY --from=0 /bin/grace /grace
COPY LICENSE README.md ./
ENTRYPOINT ["/grace"]
