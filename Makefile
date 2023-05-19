all: bin/goartrun

bin/goartrun: *.go
	mkdir -p bin
	go build -o bin/goartrun
clean:
	rm -f ./bin/goartrun
