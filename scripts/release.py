#!/usr/bin/env python3
"""Read-only preflight for publishing a stable version from the validated main commit."""
import argparse
from pathlib import Path
import re
import subprocess


VERSION = re.compile(r'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)')
COMMIT = re.compile(r'[0-9a-f]{40}')


def check_version(version):
    if not VERSION.fullmatch(version):
        raise ValueError('release version must be a stable vMAJOR.MINOR.PATCH without leading zeros')


def check_refs(raw, version, expected_commit):
    refs = {}
    for line in raw.splitlines():
        fields = line.split()
        if len(fields) != 2 or not COMMIT.fullmatch(fields[0]):
            raise ValueError('invalid remote Git reference')
        refs[fields[1]] = fields[0]
    if refs.get('refs/heads/main') != expected_commit:
        raise ValueError('remote main changed or does not match the validated commit')
    if 'refs/tags/' + version in refs:
        raise ValueError('release version already has a remote tag')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--expected-commit', required=True)
    args = parser.parse_args()
    check_version(args.version)
    if not COMMIT.fullmatch(args.expected_commit):
        raise ValueError('expected commit must be a full Git SHA')
    root = Path(__file__).resolve().parent.parent
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True, timeout=30).strip()
    if head != args.expected_commit:
        raise ValueError('checkout does not match the validated commit')
    status = subprocess.check_output(['git', 'status', '--porcelain'], cwd=root, text=True, timeout=30)
    if status:
        raise ValueError('release requires a clean worktree')
    refs = subprocess.check_output(['git', 'ls-remote', 'origin', 'refs/heads/main', 'refs/tags/' + args.version], cwd=root, text=True, timeout=30)
    check_refs(refs, args.version, args.expected_commit)
    print('Stable release preflight passed for ' + args.version + ' at ' + head)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, subprocess.CalledProcessError, subprocess.TimeoutExpired) as error:
        raise SystemExit(str(error))
