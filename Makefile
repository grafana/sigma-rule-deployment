TARGET = loki
RULE_FILE = ./test.yml

test:
	@./action.sh convert -t $(TARGET) $(RULE_FILE)

.PHONY: test
