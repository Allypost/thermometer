OUTPUT_BINARY="$(shell pwd)/bin/thermometer"

bin/thermometer:
	$(MAKE) -C go OUTPUT_BINARY="${OUTPUT_BINARY}" build

.PHONY: build
build: clean bin/thermometer

.PHONY: clean
clean:
	@rm -f "${OUTPUT_BINARY}"
