SUBDIRS := spec/ics-003-connection-semantics spec/ics-004-channel-and-packet-semantics
TOPTARGETS := all clean

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
	pandoc --pdf-engine=xelatex --template eisvogel --filter pandoc-include --mathjax --toc --number-sections -o spec.pdf spec.pdc

.PHONY: $(TOPTARGETS) $(SUBDIRS) check check_links check_dependencies check_syntax check_sections spec_pdf
