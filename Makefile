all: help

help:
	@echo Usage: make linux

darwin: 
	GOOS=darwin GOARCH=amd64 go build -o go-epg-server

linux:
	GOOS=linux GOARCH=amd64 go build -o go-epg-server

clean:
	$(RM) go-epg-server

.PHONY: clean
