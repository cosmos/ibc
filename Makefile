SUBDIRS := $(filter-out $(wildcard ./spec/*.md),$(wildcard ./spec/*))
TOPTARGETS := typecheck check_proto build clean

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
	find . -type f -name "*.md" -exec aspell -p ./misc/aspell_dict -x -d en_GB -c {} \;

spellcheck_noninteractive:
	find . -type f -name "*.md" | xargs -n 1 -I % ./scripts/spellcheck.sh %

.PHONY: $(TOPTARGETS) $(SUBDIRS) check check_links check_dependencies check_syntax check_sections check_proto spec_pdf spellcheck spellcheck_noninteractive
