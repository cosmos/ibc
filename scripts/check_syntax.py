#!/usr/bin/env python

import re, os, sys, subprocess, glob

specs = [f.path for f in os.scandir('./spec') if f.is_dir()]
files = [f.path for spec in specs for f in os.scandir(spec) if f.is_file() and f.path[-3:] == '.md']
files = sorted(files)

requires_regex = re.compile('requires: (.*)\n')

def temp_filename():
    return subprocess.check_output(['mktemp'])[:-1] + bytes('.ts', 'utf-8')

def extract_typescript(spec):
    temp = temp_filename()
    subprocess.check_output(['/bin/bash', '-c', b'cat ' + bytes(spec, 'utf-8') + b' | codedown typescript > ' + temp])
    return temp

for fn in files:
    print('Checking syntax in {}'.format(fn))
    temp = extract_typescript(fn)

    print('Running tslint...')
    output = subprocess.check_output(['tslint', '-c', './scripts/tslint.json', temp])
    if output != b'':
        sys.exit(1)

    print('Reading dependencies of {}'.format(fn))
    data = open(fn).read()
    requires = [int(num) for line in requires_regex.findall(data) for num in line.split(', ')]

    print('Dependencies: {}'.format(requires))
    final = temp_filename()

    for dep in requires:
        spec = glob.glob('./spec/ics-' + str(dep).zfill(3) + '-*')[0] + '/README.md'
        newTemp = extract_typescript(spec)
        print('Concatenating dependency on ICS {}'.format(dep))
        subprocess.check_call(['/bin/bash', '-c', b'cat ' + newTemp + b' >> ' + final])
        subprocess.check_call(['/bin/bash', '-c', b'echo -e "\n" >> ' + final])

    subprocess.check_call(['/bin/bash', '-c', b'cat ' + temp + b' >> ' + final])

    res = subprocess.run(['tsc', '--lib', 'es6', '--downlevelIteration', final])

    if res.returncode != 0:
        print(res)
        sys.exit(1)

    subprocess.check_call(['rm', temp])
