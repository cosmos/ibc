AGDADIRS := spec/ics-24-host-requirements
SUBDIRS := spec/ics-3-connection-semantics
TOPTARGETS := all clean

$(TOPTARGETS): $(SUBDIRS)
$(SUBDIRS):
	$(MAKE) -C $@ $(MAKECMDGOALS)

typecheck: $(AGDADIRS)
$(AGDADIRS):
	$(MAKE) -C $@ $(MAKECMDGOALS)

setup_dependencies:
	pip install matplotlib networkx

check_links:
	python ./scripts/check_links.py

check_dependencies:
	python ./scripts/check_dependencies.py

check_syntax:
	bash ./scripts/check_syntax.sh

check_sections:
	python ./scripts/check_sections.py

.PHONY: $(TOPTARGETS) $(SUBDIRS) $(AGDADIRS) setup_dependencies check_links check_dependencies check_syntax check_sections
