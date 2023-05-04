all: bin/goartrun

bin/goartrun: *.go
	mkdir -p bin
	go build -o bin/goartrun *.go
