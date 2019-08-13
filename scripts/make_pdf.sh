#!/bin/sh

set -xe

pandoc --pdf-engine=xelatex --template eisvogel --filter pandoc-include --mathjax --toc --number-sections -o spec.pdf spec.pdc
