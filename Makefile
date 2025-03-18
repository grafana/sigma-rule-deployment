TARGET = loki
RULE_FILE = ./test.yml

test-convert:
	@uv sync --directory actions/convert -q
	@GITHUB_WORKSPACE=$(realpath ../sigma-internal) uv run --directory actions/convert main.py --config config/sigma-convert.example.yml

test:
	@uv sync --directory actions/convert -q
	@GITHUB_WORKSPACE=$(realpath ../sigma-internal) uv run --directory actions/convert pytest -vv .

.PHONY: test test-convert
