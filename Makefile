BIN := dist

.PHONY: web build windows linux-arm64 dev-server dev-web clean

web:
	cd web && npm ci && npm run build

build: web
	go build -trimpath -ldflags "-s -w" -o $(BIN)/grimoire ./cmd/grimoire

windows: web
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $(BIN)/windows/grimoire.exe ./cmd/grimoire

linux-arm64: web
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o $(BIN)/linux-arm64/grimoire ./cmd/grimoire

dev-server:
	go run ./cmd/grimoire run -addr 127.0.0.1:7777 -data ./.devdata

dev-web:
	cd web && npm run dev

clean:
	rm -rf $(BIN) web/build/_app web/build/index.html web/build/favicon.svg
