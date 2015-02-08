build:
	go build
	go build main/consume.go

deps:
	go get github.com/streadway/amqp
	go get golang.org/x/net/context

test:
	go test -race .

travis: deps test
