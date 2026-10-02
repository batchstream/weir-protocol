#!/usr/bin/env python3
"""Enforce the standalone public module's production and test dependency graph."""
import json
import os
from pathlib import Path
import subprocess

PROTOCOL_MODULE = 'github.com/batchstream/weir-protocol'
FORBIDDEN_MODULES = {'github.com/batchstream/weir', 'github.com/batchstream/weir-go'}


def objects(raw):
    decoder = json.JSONDecoder()
    result = []
    while raw.strip():
        value, end = decoder.raw_decode(raw.lstrip())
        result.append(value)
        raw = raw.lstrip()[end:]
    return result


def check_modules(modules):
    for module in modules:
        name = module['Path']
        if name in FORBIDDEN_MODULES:
            raise ValueError('protocol module depends on Server or SDK: ' + name)
        if module.get('Replace'):
            raise ValueError('protocol module must not use replacements: ' + name)


def check_graph(raw):
    for line in raw.splitlines():
        edge = line.split()
        if len(edge) != 2:
            raise ValueError('invalid module graph edge')
        for node in edge:
            module = node.split('@', 1)[0]
            if module in FORBIDDEN_MODULES:
                raise ValueError('raw protocol graph depends on Server or SDK: ' + node)
            if module == PROTOCOL_MODULE and '@' in node:
                raise ValueError('raw protocol graph returns to a versioned protocol module: ' + node)


def check_packages(packages):
    for package in packages:
        name = package['ImportPath']
        if any(name == module or name.startswith(module + '/') for module in FORBIDDEN_MODULES):
            raise ValueError('protocol package imports Server or SDK: ' + name)


def main():
    root = Path(__file__).resolve().parent.parent
    go_bin = os.environ.get('WEIR_PROTOCOL_GO', 'go')
    go_env = dict(os.environ, GOWORK='off')
    raw_modules = subprocess.check_output([go_bin, 'list', '-m', '-json', 'all'], cwd=root, env=go_env, text=True, timeout=120)
    check_modules(objects(raw_modules))
    raw_graph = subprocess.check_output([go_bin, 'mod', 'graph'], cwd=root, env=go_env, text=True, timeout=120)
    check_graph(raw_graph)
    raw_packages = subprocess.check_output([go_bin, 'list', '-deps', '-test', '-json', './...'], cwd=root, env=go_env, text=True, timeout=120)
    check_packages(objects(raw_packages))
    print('Standalone protocol production/test module and package graph passed')


if __name__ == '__main__':
    main()
