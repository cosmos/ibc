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

pandoc --pdf-engine=xelatex --filter pandoc-include --mathjax --template=ieee-template.latex -t latex -o architecture-paper.pdf architecture-paper.pdc metadata.yaml

# cleanup

find $DIR -type f -name "*.md.xfm" -exec rm {} \;
find $DIR2 -type f -name "*.md.xfm" -exec rm {} \;
rm -f *.png *.jpg
