#!/usr/bin/env bash

set -e

for file in $(find . -type f -name "*.md"); do
  echo "Checking syntax in $file..."
  tempfile=$(mktemp).go
  echo -e "package main\n\n" > $tempfile
  cat $file | codedown golang >> $tempfile
  echo -e "\n\nfunc main() {}\n" >> $tempfile
  go run $tempfile || (cat $tempfile && rm -f $tempfile && exit 1)
  rm -f $tempfile
  cat $file | codedown coffeescript | coffee -s
done
