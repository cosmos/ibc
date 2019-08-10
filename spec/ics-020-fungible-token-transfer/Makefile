typecheck:

build:

clean:

check_proto:
	/bin/bash -c 'TEMP=$$(mktemp); protoc packets.proto -o $$TEMP; rm -f $$TEMP'

.PHONY: typecheck build clean check_proto
