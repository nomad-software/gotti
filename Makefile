BINARY := gotti

# -s -w leaves out the symbol table and debug information, which roughly
# halves the size of the binary.
LDFLAGS := -s -w

.PHONY: build run test fuzz bench vet icons screenshot clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/gotti
	go build -ldflags "$(LDFLAGS)" -o bin/gotti-headless ./cmd/gotti-headless

run: build
	./bin/$(BINARY)

# Every run of the tests has a timeout, so that one that hangs fails with
# the stacks of what it was waiting on, rather than waiting for ever.
TEST_TIMEOUT := 5m

test:
	go test -race -timeout $(TEST_TIMEOUT) ./...

fuzz:
	go test -run '^$$' -fuzz FuzzParser -fuzztime 60s ./internal/term

bench:
	go test -timeout $(TEST_TIMEOUT) -run '^$$' -bench . ./internal/term

vet:
	go vet ./...

# Draws the program's icon, internal/appicon/appicon.png, from
# packaging/gotti.svg. Run it again whenever the SVG changes.
icons:
	rsvg-convert -w 256 -h 256 packaging/gotti.svg -o internal/appicon/appicon.png

# Draws the README screenshot, docs/screenshot.png, from a demo layout in a
# virtual X server. Needs Xvfb, ImageMagick and vim.
screenshot:
	packaging/screenshot.sh

clean:
	rm -rf bin
