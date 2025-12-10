ANOLIS_DIST=dist/anolis
CGO_ENABLED=0

.PHONY: build-linux-anolis-amd64 build-linux-anolis-arm64 build-linux-anolis-all package-anolis-amd64 package-anolis-arm64 package-anolis-all clean

build-linux-anolis-amd64:
	mkdir -p $(ANOLIS_DIST)/amd64/auto-https/bin $(ANOLIS_DIST)/amd64/auto-https/state
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build -o $(ANOLIS_DIST)/amd64/auto-https/bin/rotate-cert ./cmd/rotate
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build -o $(ANOLIS_DIST)/amd64/auto-https/bin/alidns-update ./
	cp README.md $(ANOLIS_DIST)/amd64/auto-https/
	printf '{\n  "last_replace_unix": 0\n}\n' > $(ANOLIS_DIST)/amd64/auto-https/state/state.json

build-linux-anolis-arm64:
	mkdir -p $(ANOLIS_DIST)/arm64/auto-https/bin $(ANOLIS_DIST)/arm64/auto-https/state
	GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) go build -o $(ANOLIS_DIST)/arm64/auto-https/bin/rotate-cert ./cmd/rotate
	GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) go build -o $(ANOLIS_DIST)/arm64/auto-https/bin/alidns-update ./
	cp README.md $(ANOLIS_DIST)/arm64/auto-https/
	printf '{\n  "last_replace_unix": 0\n}\n' > $(ANOLIS_DIST)/arm64/auto-https/state/state.json

build-linux-anolis-all: build-linux-anolis-amd64 build-linux-anolis-arm64

package-anolis-amd64: build-linux-anolis-amd64
	mkdir -p $(ANOLIS_DIST)/pkg
	cd $(ANOLIS_DIST)/amd64 && tar -czf ../pkg/auto-https-anolis8.5-amd64.tar.gz auto-https

package-anolis-arm64: build-linux-anolis-arm64
	mkdir -p $(ANOLIS_DIST)/pkg
	cd $(ANOLIS_DIST)/arm64 && tar -czf ../pkg/auto-https-anolis8.5-arm64.tar.gz auto-https

package-anolis-all: package-anolis-amd64 package-anolis-arm64

clean:
	rm -rf $(ANOLIS_DIST)
