BINARY := anime-tracker
DIST   := dist

.PHONY: build run test vet tidy clean cross

build:
	go build -o $(BINARY) .

run: build
	./$(BINARY)

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf $(BINARY) $(DIST)

cross:
	mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -o $(DIST)/$(BINARY)-linux-amd64 .
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o $(DIST)/$(BINARY)-windows-amd64.exe .
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -o $(DIST)/$(BINARY)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -o $(DIST)/$(BINARY)-darwin-arm64 .
