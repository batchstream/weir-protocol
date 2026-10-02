"""Offline dependency checker tests; no Go, GitHub, or module requests."""
import json
import unittest

import check_dependencies


class DependencyTests(unittest.TestCase):
    def test_protocol_and_runtime_modules_are_allowed(self):
        modules = [{'Path': path} for path in ('github.com/batchstream/weir-protocol', 'google.golang.org/grpc', 'golang.org/x/net')]
        check_dependencies.check_modules(modules)

    def test_any_server_or_sdk_requirement_is_rejected(self):
        for path in check_dependencies.FORBIDDEN_MODULES:
            modules = [{'Path': 'github.com/batchstream/weir-protocol'}, {'Path': path, 'Indirect': True}]
            with self.assertRaisesRegex(ValueError, 'Server or SDK'):
                check_dependencies.check_modules(modules)

    def test_any_replacement_is_rejected(self):
        modules = [{'Path': 'example.com/runtime', 'Replace': {'Dir': '/local'}}]
        with self.assertRaisesRegex(ValueError, 'replacements'):
            check_dependencies.check_modules(modules)

    def test_test_packages_cannot_import_server_or_sdk(self):
        for path in ('github.com/batchstream/weir/internal/testutil', 'github.com/batchstream/weir-go/internal/testfixture', 'github.com/batchstream/weir/api/protocol'):
            packages = [{'ImportPath': path}]
            with self.assertRaisesRegex(ValueError, 'Server or SDK'):
                check_dependencies.check_packages(packages)

    def test_own_fixture_and_api_are_allowed(self):
        packages = [{'ImportPath': path} for path in ('github.com/batchstream/weir-protocol/api/protocol', 'github.com/batchstream/weir-protocol/internal/testutil/testdns', 'context')]
        check_dependencies.check_packages(packages)

    def test_go_json_objects_decode_without_wrapper_array(self):
        expected = [{'Path': 'github.com/batchstream/weir-protocol'}, {'Path': 'google.golang.org/grpc'}]
        raw = '\n'.join(json.dumps(item) for item in expected)
        self.assertEqual(check_dependencies.objects(raw), expected)


if __name__ == '__main__':
    unittest.main()
