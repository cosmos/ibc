#!/bin/bash

lines=$(cat $1 | aspell -p ./misc/aspell_dict -x -d en_GB list)
if [[ -z "$lines" ]]; then
  exit 0
fi
exit 1
