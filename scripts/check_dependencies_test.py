"""Offline dependency checker tests; no Go, GitHub, or module requests."""
import json
import os
import unittest
from unittest.mock import patch

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

    def test_raw_graph_rejects_unused_and_old_server_sdk_edges(self):
        for edge in (
            'example.com/dep@v1.0.0 github.com/batchstream/weir@v0.1.0',
            'github.com/batchstream/weir-go@v0.1.0 example.com/dep@v2.0.0',
            'github.com/batchstream/weir-protocol github.com/batchstream/weir-go@v0.0.1',
        ):
            with self.assertRaisesRegex(ValueError, 'Server or SDK'):
                check_dependencies.check_graph(edge)

    def test_raw_graph_rejects_versioned_protocol_return_edges(self):
        for edge in (
            'example.com/dep@v1.0.0 github.com/batchstream/weir-protocol@v0.0.1',
            'github.com/batchstream/weir-protocol@v0.0.1 example.com/dep@v1.0.0',
        ):
            with self.assertRaisesRegex(ValueError, 'versioned protocol'):
                check_dependencies.check_graph(edge)

    def test_raw_graph_accepts_root_and_external_versions(self):
        graph = 'github.com/batchstream/weir-protocol google.golang.org/grpc@v1.83.2\ngoogle.golang.org/grpc@v1.83.2 google.golang.org/protobuf@v1.36.11\n'
        check_dependencies.check_graph(graph)

    def test_go_subprocesses_disable_an_inherited_workspace(self):
        modules = json.dumps({'Path': check_dependencies.PROTOCOL_MODULE})
        graph = check_dependencies.PROTOCOL_MODULE + ' google.golang.org/grpc@v1.83.2'
        packages = json.dumps({'ImportPath': check_dependencies.PROTOCOL_MODULE + '/api/protocol'})
        environment = {'GOWORK': '/unrelated/go.work'}
        with patch.dict(os.environ, environment), patch.object(check_dependencies.subprocess, 'check_output', side_effect=[modules, graph, packages]) as command:
            check_dependencies.main()
        self.assertEqual(command.call_count, 3)
        for call in command.call_args_list:
            self.assertEqual(call.kwargs['env']['GOWORK'], 'off')

    def test_go_json_objects_decode_without_wrapper_array(self):
        expected = [{'Path': 'github.com/batchstream/weir-protocol'}, {'Path': 'google.golang.org/grpc'}]
        raw = '\n'.join(json.dumps(item) for item in expected)
        self.assertEqual(check_dependencies.objects(raw), expected)


if __name__ == '__main__':
    unittest.main()
