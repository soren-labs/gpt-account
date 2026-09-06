GO ?= $(HOME)/.local/go/bin/go

.PHONY: test build install manager

test:
	$(GO) test ./...
	cmp -s agent/gpa_agent.py skills/gpa-account-manager/scripts/gpa_agent.py

build:
	mkdir -p dist
	$(GO) build -o dist/gpa ./cmd/gpa
	$(GO) build -o dist/gpa-manager ./cmd/gpa-manager
	GOOS=windows GOARCH=amd64 $(GO) build -o dist/gpa.exe ./cmd/gpa
	GOOS=windows GOARCH=amd64 $(GO) build -o dist/gpa-manager.exe ./cmd/gpa-manager

install: build
	./dist/gpa install --linux-bin dist/gpa --windows-bin dist/gpa.exe

manager: build
	./dist/gpa-manager --demo --no-browser --listen 127.0.0.1:18765 --test-session demoboot --store $(CURDIR)/.demo-store
