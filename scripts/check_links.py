#!/usr/bin/env python

import re, os, sys

link_regex = re.compile('\[(.*)\]\(../ics([^\)]*)\)')

specs = [f.path for f in os.scandir('./spec') if f.is_dir()]
files = [f.path for spec in specs for f in os.scandir(spec) if f.is_file() and f.path[-3:] == '.md']

specs_cut = set([spec[7:] for spec in specs])

for fn in files:
    print('Checking links in {}'.format(fn))
    data = open(fn).read()
    links = ['ics' + l[1] for l in link_regex.findall(data)]
    for link in links:
        found = link in specs_cut
        if not found:
            print('Link to {} not found!'.format(link))
            sys.exit(1)
