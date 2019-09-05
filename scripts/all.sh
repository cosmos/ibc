#!/bin/bash

set -xe

make check
make typecheck
make spellcheck
make check_proto
make build
make spec_pdf
