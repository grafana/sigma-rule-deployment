TARGET = loki
RULE_FILE = ./test.yml

test-convert:
	@uv sync -q
	@GITHUB_WORKSPACE=$(realpath ../sigma-internal) uv run actions/convert/main.py --config config/sigma-convert.example.yml

test:
	@uv sync -q
	@GITHUB_WORKSPACE=$(realpath ../sigma-internal) uv run pytest -v .

.PHONY: test test-convert
