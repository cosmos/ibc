#!/usr/bin/env bash

set -e

for file in $(find . -type f -name "*.md"); do
  echo "Checking syntax in $file..."
  cat $file | codedown coffeescript | coffee -s
done
