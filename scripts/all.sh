#!/bin/bash

set -xe

make check
make typecheck
make spellcheck
make build
make spec_pdf
