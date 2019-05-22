#!/usr/bin/env bash

set -e

for file in $(find . -type f -name "*.md"); do
  echo "Checking syntax in $file..."
  tempfile=$(mktemp).ts
  cat $file | codedown typescript > $tempfile
  cat $tempfile
  echo "Running tslint..."
  tslint -c ./scripts/tslint.json $tempfile
  echo "Running typescript compiler (note: not required for the time being)..."
  (tsc --lib es6 --downlevelIteration $tempfile || exit 0)
  rm -f $tempfile
done
