typecheck:

build: $(addsuffix .png, $(basename $(wildcard *.tex))) cleanup

%.png : %.tex
	pdflatex "\input{$<}"
	convert -resize 50% -chop 26x0 -density 400 $(basename $<).pdf -quality 100 $@

cleanup:
	rm -f *.log
	rm -f *.aux
	rm -f *.pdf

clean:
	rm -f *.png

check_proto:
	/bin/bash -c 'TEMP=$$(mktemp); protoc data-structures.proto -o $$TEMP; rm -f $$TEMP'

.PHONY: typecheck build clean cleanup check_proto
