SUBDIRS := $(filter-out $(wildcard ./spec/*.md),$(wildcard ./spec/*))
TOPTARGETS := typecheck build clean

$(TOPTARGETS): $(SUBDIRS)
$(SUBDIRS):
	$(MAKE) -C $@ $(MAKECMDGOALS)

check: check_links check_dependencies check_syntax check_sections check_proto

check_links:
	python ./scripts/check_links.py

check_dependencies:
	python ./scripts/check_dependencies.py

check_syntax:
	python ./scripts/check_syntax.py

check_sections:
	python ./scripts/check_sections.py

check_proto:
	$(MAKE) -C spec/ics-002-consensus-verification check_proto
	$(MAKE) -C spec/ics-003-connection-semantics check_proto
	$(MAKE) -C spec/ics-004-channel-and-packet-semantics check_proto
	$(MAKE) -C spec/ics-020-fungible-token-transfer check_proto
	$(MAKE) -C spec/ics-026-relayer-module check_proto

spec_pdf:
	pandoc --pdf-engine=xelatex --template eisvogel --filter pandoc-include --mathjax --toc --number-sections -o spec.pdf spec.pdc

spellcheck:
	find . -type f -name "*.md" -exec aspell -p ./misc/aspell_dict -x -d en_GB -c {} \;

spellcheck_noninteractive:
	find . -type f -name "*.md" | xargs -n 1 -I % ./scripts/spellcheck.sh %

.PHONY: $(TOPTARGETS) $(SUBDIRS) check check_links check_dependencies check_syntax check_sections check_proto spec_pdf spellcheck spellcheck_noninteractive
