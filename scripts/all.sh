#!/bin/bash

set -xe

make check
make spellcheck
make build
make spec_pdf
