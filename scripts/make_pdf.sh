#!/bin/sh

set -xe

DIR=./spec
DIR2=./ibc

# preprocessing

find $DIR -type f -name "*.md" -exec cp {} {}.xfm \;
find $DIR -type f -name "*.md.xfm" -exec awk -i inplace '/## Backwards Compatibility/ {exit} {print}' {} \;
find $DIR -type f -name "*.png" -exec cp {} . \;
find $DIR -type f -name "*.jpg" -exec cp {} . \;
find $DIR2 -type f -name "*.md" -exec cp {} {}.xfm \;
find $DIR2 -type f -name "*.md.xfm" -exec awk -i inplace '/^##/{p=1}p' {} \;

# pdf generation

pandoc --pdf-engine=xelatex --template eisvogel --filter pandoc-include --mathjax --toc --number-sections --listings -t latex -o spec.pdf spec.pdc

# cleanup

find $DIR -type f -name "*.md.xfm" -exec rm {} \;
find $DIR2 -type f -name "*.md.xfm" -exec rm {} \;
rm -f *.png *.jpg
