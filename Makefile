TARGET = loki
RULE_FILE = ./test.yml

test:
	@./action.sh convert -t $(TARGET) $(RULE_FILE)

test-convert:
	@uv sync --directory actions/convert -q
	@GITHUB_WORKSPACE=$(realpath ../sigma-internal) uv run --directory actions/convert main.py --config ../../config/sigma-convert.example.yml

.PHONY: test test-convert
