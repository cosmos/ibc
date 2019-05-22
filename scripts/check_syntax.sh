#!/usr/bin/env bash

set -e

for file in $(find ./spec -type f -name "*.md"); do
  echo "Checking syntax in $file..."
  tempfile=$(mktemp).ts
  cat $file | codedown typescript > $tempfile
  echo "Running tslint..."
  tslint -c ./scripts/tslint.json $tempfile || (cat $tempfile && exit 1)
  echo "Running typescript compiler..."
  tsc --lib es6 --downlevelIteration $tempfile || (cat $tempfile)
  rm -f $tempfile
done
