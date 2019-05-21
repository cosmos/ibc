#!/usr/bin/env python

import re, os, sys

top_section_regex = re.compile('^# (.*)$')
sub_section_regex = re.compile('^## (.*)$')
sub_sub_section_regex = re.compile('^### (.*)$')

specs = [f.path for f in os.scandir('./spec') if f.is_dir()]
files = [f.path for spec in specs for f in os.scandir(spec) if f.is_file() and f.path[-3:] == '.md' and 'ics-1' not in f.path]

for fn in files:
    print('Checking sections in {}'.format(fn))
    data = open(fn).read()
    print(data)
    top_sections = top_section_regex.findall(data)
    print('top', top_sections)
    sub_sections = sub_section_regex.findall(data)
    print('sub', sub_sections)
    sub_sub_sections = sub_sub_section_regex.findall(data)
    print('subsub', sub_sub_sections)
