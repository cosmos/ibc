all: $(filter-out common.png, $(addsuffix .png, $(basename $(wildcard *.tex)))) cleanup

%.png : %.tex common.tex
	pdflatex "\input{$<}"
	convert -resize 50% -chop 26x0 -density 400 $(basename $<).pdf -quality 100 $@
	mv $(basename $@)-1.png $(basename $@).png
	rm $(basename $@)-0.png

cleanup:
	rm -f *.log
	rm -f *.aux
	rm -f *.pdf

.PHONY: all cleanup
