all: $(filter-out common.png, $(addsuffix .png, $(basename $(wildcard *.tex)))) cleanup

%.png : %.tex
	pdflatex "\input{$<}"
	convert -density 300 $(basename $<).pdf -quality 90 $@
	mv $(basename $@)-1.png $(basename $@).png
	rm $(basename $@)-0.png

cleanup:
	rm -f *.log
	rm -f *.aux
	rm -f *.pdf

.PHONY: all cleanup
