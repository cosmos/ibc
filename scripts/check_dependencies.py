#!/usr/bin/env python

import networkx as nx
import matplotlib.pyplot as plt
import re, os, sys

G = nx.DiGraph()

specs = [f.path for f in os.scandir('./spec') if f.is_dir()]
files = [f.path for spec in specs for f in os.scandir(spec) if f.is_file() and f.path[-3:] == '.md']

origin_regex = re.compile('./spec/ics-([0-9]*)-(.*)')
requires_regex = re.compile('requires: (.*)\n')
required_regex = re.compile('required-by: (.*)\n')

print('Constructing directed dependency graph...')

all_required_by = []

for fn in files:
    print('Reading dependencies of {}'.format(fn))
    data = open(fn).read()
    origin = int(origin_regex.findall(fn)[0][0])
    G.add_node(origin)
    requires = [int(num) for line in requires_regex.findall(data) for num in line.split(', ')]
    for num in requires:
        G.add_edge(origin, num)
    required_by = [int(num) for line in required_regex.findall(data) for num in line.split(', ')]
    for num in required_by:
        all_required_by.append((origin, num))

edges = list(G.edges)

for (num, origin) in all_required_by:
    edge = (origin, num)
    if edge not in edges:
        print('Missing requirement from {} to {}!'.format(origin, num))
        sys.exit(1)

print('Scanning for possible cycles...')

cycles = list(nx.algorithms.cycles.simple_cycles(G))

if len(cycles) == 0:
    print('No cycles!')
else:
    print('Found cycles!')
    print(cycles)

print('Drawing dependency graph...')

nx.draw_circular(G, with_labels = True, font_weight = 'bold', node_size = 500)
plt.savefig('assets/deps.png')
