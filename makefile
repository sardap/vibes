.DEFAULT_GOAL := build

.PHONY: build
build:
	docker build . -t vibes:latest
	
.PHONY: run
run:
	docker-compose build
	docker-compose up -d
	@echo "running localy go to localhost:5000"

.PHONY: shutdown
shutdown:
	docker-compose down

.PHONY: restart
restart: run_local shutdown