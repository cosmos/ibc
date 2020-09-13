all: clean_temp
	./scripts/all.sh

SUBDIRS := $(filter-out $(wildcard ./spec/*.md),$(wildcard ./spec/*))
TOPTARGETS := build clean

$(TOPTARGETS): $(SUBDIRS)
$(SUBDIRS):
	$(MAKE) -C $@ $(MAKECMDGOALS)

check: check_links check_dependencies check_syntax check_sections

check_links:
	python ./scripts/check_links.py

check_dependencies:
	python ./scripts/check_dependencies.py

check_syntax:
	python ./scripts/check_syntax.py

check_sections:
	python ./scripts/check_sections.py

spec_pdf:
	scripts/make_pdf.sh

spellcheck:
	find ./spec ./ibc -type f -name "*.md" -exec aspell -p ./misc/aspell_dict -x -d en_GB -c {} \;

spellcheck_noninteractive:
	find ./spec ./ibc -type f -name "*.md" | xargs -n 1 -I % ./scripts/spellcheck.sh %

clean_temp:
	rm -f spec/ics-template.md.xfm
	rm -f *.png
	find ./ -type f -name '*.temp.pandoc-include' | xargs -n 1 -I % rm %

# due to https://github.com/golang/protobuf/issues/39 this requires multiple commands
protoc:
	protoc --go_out=compliance/shims/go `find ./spec/ics-002-client-semantics -type f -name "*.proto"`
	protoc --go_out=compliance/shims/go `find ./spec/ics-003-connection-semantics -type f -name "*.proto"`
	protoc --go_out=compliance/shims/go `find ./spec/ics-004-channel-and-packet-semantics -type f -name "*.proto"`
	protoc --go_out=compliance/shims/go `find ./spec/ics-020-fungible-token-transfer -type f -name "*.proto"`

.PHONY: $(TOPTARGETS) $(SUBDIRS) all check check_links check_dependencies check_syntax check_sections spec_pdf spellcheck spellcheck_noninteractive
