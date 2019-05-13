SUBDIRS := spec/ics-3-bidirectional-connection-semantics
TOPTARGETS := all clean

$(TOPTARGETS): $(SUBDIRS)
$(SUBDIRS):
	$(MAKE) -C $@ $(MAKECMDGOALS)

.PHONY: $(TOPTARGETS) $(SUBDIRS)
