#!/usr/bin/env python

import re, os, sys

top_section_regex = re.compile('[^#]# (.*)')
sub_section_regex = re.compile('[^#]## (.*)')
sub_sub_section_regex = re.compile('[^#]### (.*)')

specs = [f.path for f in os.scandir('./spec') if f.is_dir()]
files = [f.path for spec in specs for f in os.scandir(spec) if f.is_file() and f.path == 'README.md' and 'ics-1-ics-standard' not in f.path]

expected_sub_sections = ['Synopsis', 'Technical Specification', 'Backwards Compatibility', 'Forwards Compatibility', 'Example Implementation', 'Other Implementations', 'History', 'Copyright']
expected_sub_sub_sections = ['Motivation', 'Definitions', 'Desired Properties']

for fn in files:
    print('Checking sections in {}'.format(fn))
    data = open(fn).read()
    top_sections = top_section_regex.findall(data)
    if len(top_sections) != 0:
        print('Expected no top-level sections but instead found: {}'.format(top_sections))
        sys.exit(1)
    sub_sections = sub_section_regex.findall(data)[::-1]
    for sub_section in expected_sub_sections:
        found = sub_sections.pop()
        if sub_section != found:
            print('Expected sub-section {} but instead found {}!'.format(sub_section, found))
            sys.exit(1)
    if len(sub_sections) != 0:
        print('Expected no remaining sub-sections but instead found: {}'.format(sub_sections))
        sys.exit(1)
    sub_sub_sections = sub_sub_section_regex.findall(data)[::-1]
    for sub_sub_section in expected_sub_sub_sections:
        found = sub_sub_sections.pop()
        if sub_sub_section != found:
            print('Expected sub-sub-section {} but instead found {}!'.format(sub_section, found))
            sys.exit(1)
    print('Remaining sub-sub-sections: {}'.format(sub_sub_sections))
