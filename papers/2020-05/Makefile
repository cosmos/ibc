FILES = src/paper.md \
		src/metadata.yaml

OUTPUT = build

FLAGS = --bibliography=src/bibliography.bib \
		--csl=src/bibliography.csl \
		--filter pandoc-include \
		-s \
		-f markdown

FLAGS_PDF = --template=src/template.latex

all: pdf

pdf:
	pandoc -o $(OUTPUT)/paper.pdf $(FLAGS) $(FLAGS_PDF) $(FILES)

wc:
	wc src/*.md

clean:
	rm -rf build/*

.PHONY: all pdf wc clean
