TARGET = loki
RULE_FILE = ./test.yml

test-convert:
	@uv sync --prerelease=allow --directory actions/convert -q
	@GITHUB_WORKSPACE=$(realpath ../sigma-internal) uv run --prerelease=allow --directory actions/convert main.py --config config/sigma-convert.example.yml

test:
	@uv sync --prerelease=allow --directory actions/convert -q
	@GITHUB_WORKSPACE=$(realpath ../sigma-internal) uv run --prerelease=allow --directory actions/convert pytest -vv .

.PHONY: test test-convert
