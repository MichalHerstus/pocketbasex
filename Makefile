BINARY := pbx
HTTP_ADDR := 127.0.0.1:8090

# Target platform: one of macOS (default), windows, linux.
# Build goes to pbx-<OS> (.exe on windows). Default variant is macOS/arm64.
OS ?= macOS

# OS/arch mapping.
GOOS   ?= $(if $(filter windows,$(OS)),windows,$(if $(filter linux,$(OS)),linux,darwin))
GOARCH ?= $(if $(filter windows,$(OS)),amd64,$(if $(filter linux,$(OS)),amd64,arm64))
EXT    := $(if $(filter windows,$(OS)),.exe,)

OUT := $(BINARY)-$(OS)$(EXT)

.PHONY: build run stop vet clean

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o $(OUT) .
	go vet ./...

run: build
	./$(OUT) serve --http $(HTTP_ADDR)

stop:
	pkill -f "$(OUT) serve"; sleep 1; pgrep -fl "$(OUT) serve" || echo "$(OUT) stopped"

vet:
	go vet ./...

clean:
	rm -f $(BINARY)-*