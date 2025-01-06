IMAGE_NAME = sigma-cli:latest
RULES_DIR = $(PWD)
RULE_FILE = /rules/test.yml
TARGET = loki

# Build the Docker image without using the cache
build:
	@docker build --no-cache -t $(IMAGE_NAME) .

# Run the sigma-cli container with the /rules directory mounted
test:
	@docker run --rm -v $(RULES_DIR):/rules $(IMAGE_NAME) convert -t $(TARGET) $(RULE_FILE)

.PHONY: build test
