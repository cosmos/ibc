SUBDIRS := spec/ics-3-connection-semantics
TOPTARGETS := all clean

$(TOPTARGETS): $(SUBDIRS)
$(SUBDIRS):
	$(MAKE) -C $@ $(MAKECMDGOALS)

check_links:
	python ./scripts/check_links.py

check_dependencies:
	python ./scripts/check_dependencies.py

check_syntax:
	python ./scripts/check_syntax.py

check_sections:
	python ./scripts/check_sections.py

.PHONY: $(TOPTARGETS) $(SUBDIRS) check_links check_dependencies check_syntax check_sections
