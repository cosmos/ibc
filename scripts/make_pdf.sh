#!/bin/sh

set -xe

DIR=./spec
find $DIR -type f -name "*.md" -exec cp {} {}.xfm \;
find $DIR -type f -name "*.md.xfm" -exec awk -i inplace '/## Backwards Compatibility/ {exit} {print}' {} \;
find $DIR -type f -name "*.png" -exec cp {} . \;
pandoc --pdf-engine=xelatex --template eisvogel --filter pandoc-include --mathjax --toc --number-sections -o spec.pdf spec.pdc
rm *.png
