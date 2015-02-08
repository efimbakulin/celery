build:
	go build -race
	go build -race main/consume.go

deps:
	go get github.com/streadway/amqp
	go get golang.org/x/net/context

test:
	go test -v -race .

travis: deps test
