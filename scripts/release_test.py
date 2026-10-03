"""Release preflight tests without publishing tags or contacting remote services."""
import sys
import unittest
from unittest.mock import patch

import release


COMMIT = 'a' * 40


class ReleaseTests(unittest.TestCase):
    def test_stable_versions(self):
        for version in ('v0.1.0', 'v1.0.0', 'v12.34.56'):
            release.check_version(version)

    def test_nonstable_or_ambiguous_versions_are_rejected(self):
        for version in ('main', '0.1.0', 'v0.1', 'v0.1.0-rc.1', 'v0.1.0+build', 'v01.1.0', 'v0.01.0', 'v0.1.00', 'v0.1.0\n', 'v0.1.0;command'):
            with self.assertRaises(ValueError):
                release.check_version(version)

    def test_exact_current_main_without_existing_tag(self):
        release.check_refs(COMMIT + '\trefs/heads/main\n', 'v0.1.0', COMMIT)

    def test_missing_or_changed_main_is_rejected(self):
        for refs in ('', 'b' * 40 + '\trefs/heads/main\n'):
            with self.assertRaisesRegex(ValueError, 'remote main'):
                release.check_refs(refs, 'v0.1.0', COMMIT)

    def test_existing_remote_version_is_rejected(self):
        refs = COMMIT + '\trefs/heads/main\n' + 'b' * 40 + '\trefs/tags/v0.1.0\n'
        with self.assertRaisesRegex(ValueError, 'already has'):
            release.check_refs(refs, 'v0.1.0', COMMIT)

    def test_invalid_remote_output_is_rejected(self):
        for refs in ('invalid', 'HEAD refs/heads/main', COMMIT + ' refs/heads/main extra'):
            with self.assertRaisesRegex(ValueError, 'invalid remote'):
                release.check_refs(refs, 'v0.1.0', COMMIT)

    def test_preflight_only_runs_read_only_git_commands(self):
        args = ['release.py', '--version', 'v0.1.0', '--expected-commit', COMMIT]
        replies = [COMMIT + '\n', '', COMMIT + '\trefs/heads/main\n']
        with patch.object(sys, 'argv', args), patch.object(release.subprocess, 'check_output', side_effect=replies) as command:
            release.main()
        self.assertEqual(command.call_count, 3)
        self.assertEqual(command.call_args_list[0].args[0], ['git', 'rev-parse', 'HEAD'])
        self.assertEqual(command.call_args_list[1].args[0], ['git', 'status', '--porcelain'])
        self.assertEqual(command.call_args_list[2].args[0], ['git', 'ls-remote', 'origin', 'refs/heads/main', 'refs/tags/v0.1.0'])

    def test_invalid_version_stops_before_git(self):
        args = ['release.py', '--version', 'main', '--expected-commit', COMMIT]
        with patch.object(sys, 'argv', args), patch.object(release.subprocess, 'check_output') as command:
            with self.assertRaisesRegex(ValueError, 'stable'):
                release.main()
        command.assert_not_called()

    def test_invalid_commit_stops_before_git(self):
        args = ['release.py', '--version', 'v0.1.0', '--expected-commit', 'abc']
        with patch.object(sys, 'argv', args), patch.object(release.subprocess, 'check_output') as command:
            with self.assertRaisesRegex(ValueError, 'full Git SHA'):
                release.main()
        command.assert_not_called()

    def test_dirty_checkout_stops_before_remote_lookup(self):
        args = ['release.py', '--version', 'v0.1.0', '--expected-commit', COMMIT]
        with patch.object(sys, 'argv', args), patch.object(release.subprocess, 'check_output', side_effect=[COMMIT + '\n', ' M source.go\n']) as command:
            with self.assertRaisesRegex(ValueError, 'clean worktree'):
                release.main()
        self.assertEqual(command.call_count, 2)

    def test_wrong_checkout_stops_before_remote_lookup(self):
        args = ['release.py', '--version', 'v0.1.0', '--expected-commit', COMMIT]
        with patch.object(sys, 'argv', args), patch.object(release.subprocess, 'check_output', return_value='b' * 40 + '\n') as command:
            with self.assertRaisesRegex(ValueError, 'checkout'):
                release.main()
        self.assertEqual(command.call_count, 1)


if __name__ == '__main__':
    unittest.main()
