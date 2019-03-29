all: $(addsuffix .png, $(basename $(wildcard *.tex))) cleanup

%.png : %.tex
	pdflatex "\input{$<}"
	convert -density 300 $(basename $<).pdf -quality 90 $@

cleanup:
	rm -f *.log
	rm -f *.aux
	rm -f *.pdf

.PHONY: all cleanup
