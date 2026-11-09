BINARY := termite

# -s -w leaves out the symbol table and debug information, which roughly
# halves the size of the binary.
LDFLAGS := -s -w

.PHONY: build run test fuzz bench guibench vet icons screenshot appimage clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .
	go build -ldflags "$(LDFLAGS)" -o test/termite-headless ./test/headless

run: build
	./$(BINARY)

# Every run of the tests has a timeout, so that one that hangs fails with
# the stacks of what it was waiting on, rather than waiting for ever.
TEST_TIMEOUT := 5m

test:
	go test -race -timeout $(TEST_TIMEOUT) ./...

fuzz:
	go test -run '^$$' -fuzz FuzzParser -fuzztime 60s ./internal/term

bench:
	go test -timeout $(TEST_TIMEOUT) -run '^$$' -bench . ./internal/term

# Throughput, frame costs and a CPU profile of the GUI under Xvfb; see
# test/guibench/run.sh. Run it on two builds to compare them.
guibench: build
	test/guibench/run.sh ./$(BINARY)

vet:
	go vet ./...

# Draws the program's icon, internal/appicon/appicon.png, from
# packaging/termite.svg. Run it again whenever the SVG changes.
icons:
	rsvg-convert -w 256 -h 256 packaging/termite.svg -o internal/appicon/appicon.png

# Builds Termite-<version>-x86_64.AppImage here with the Qt found by
# pkg-config (see packaging/appimage.sh). The work is done in a temporary
# folder outside the project, which is removed afterwards. Releases are
# built by .github/workflows/appimage.yml on Ubuntu 22.04, so they run on
# older systems than one built here would.
appimage:
	packaging/appimage.sh

# Draws the README screenshot, docs/screenshot.png, from a demo layout in a
# virtual X server. Needs Xvfb, ImageMagick and vim.
screenshot:
	packaging/screenshot.sh

clean:
	rm -f $(BINARY) test/termite-headless Termite-*.AppImage
