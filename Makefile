BINARY := pbx
HTTP_ADDR := 127.0.0.1:8090

.PHONY: build run stop vet clean

build:
	go build -o $(BINARY) .
	go vet ./...

run: build
	./$(BINARY) serve --http $(HTTP_ADDR)

stop:
	pkill -f "$(BINARY) serve"; sleep 1; pgrep -fl "$(BINARY) serve" || echo "$(BINARY) stopped"

vet:
	go vet ./...

clean:
	rm -f $(BINARY)