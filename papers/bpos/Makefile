all: $(addsuffix .pdf, $(basename $(wildcard *.pdc)))

%.pdf: %.pdc
	pandoc --pdf-engine=xelatex --mathjax --toc --number-sections -o $@ $<

clean:
	rm *.pdf

.PHONY: clean
