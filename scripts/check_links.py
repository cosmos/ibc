#!/usr/bin/env python

import re, os, sys

link_regex = re.compile('\[ICS ([0-9]+)\]\(([^\)]*)\)')
title_regex = re.compile('ICS ([0-9]+)([ .:])')

specs = [f.path for f in os.scandir('./spec') if f.is_dir()]
files = [f.path for spec in specs for f in os.scandir(spec) if f.is_file() and f.path[-3:] == '.md']

specs_cut = set([spec[7:] for spec in specs])

for fn in files:
    print('Checking links in {}'.format(fn))
    data = open(fn).read()
    links = [l[1][3:] for l in link_regex.findall(data)]
    for link in links:
        found = link in specs_cut
        if not found:
            print('Link to {} not found!'.format(link))
            sys.exit(1)
    titles = [int(x[0]) for x in title_regex.findall(data)]
    for num in titles:
        matched = [f for f in files if f[7:7+4+len(str(num))+1] == 'ics-' + str(num) + '-']
        if len(matched) > 0:
            print('Expected "ICS {}" to link to {} but not found!'.format(num, matched[0]))
            sys.exit(1)
