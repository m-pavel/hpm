# Variables
APP_NAME := hpm
DOCKER_IMAGE := hpm:latest

# Build the Go binary
build:
	go build -o $(APP_NAME) ./cmd

# Build the Docker image
docker-build: build
	docker build -t $(DOCKER_IMAGE) .

# Clean up
clean:
	rm -f $(APP_NAME)

run: docker-build
	docker compose up -d 

.PHONY: build docker-build clean