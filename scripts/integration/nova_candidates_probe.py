#!/usr/bin/env python3
"""Use the deployed Nova report client/config to query real Placement candidates.

This is a client probe, not a scheduler boot. B14 separately requires server create.
"""
import argparse
import json
import logging
from pathlib import Path
import sys
import traceback


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--provider-id', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=False)
    logging.disable(logging.CRITICAL)
    try:
        from nova import config, context, objects
        from nova.scheduler import utils
        from nova.scheduler.client import report
        config.parse_args(['nova-candidates-probe', '--config-file', '/etc/nova/nova.conf'], configure_db=False, init_rpc=False)
        objects.register_all()
        flavor = objects.Flavor(vcpus=1, memory_mb=512, root_gb=1,
                                ephemeral_gb=0, swap=0, extra_specs={})
        params = objects.RequestLevelParams(root_required={'OWNER_NOVA'},
                                            root_forbidden={'COMPUTE_STATUS_DISABLED'}, same_subtree=[])
        spec = objects.RequestSpec(flavor=flavor, request_level_params=params,
                                   requested_resources=[], image=objects.ImageMeta(properties=objects.ImageMetaProps()))
        request = utils.ResourceRequest.from_request_spec(spec)
        ctx = context.get_admin_context()
        candidates, summaries, version = report.report_client_singleton().get_allocation_candidates(ctx, request)
        result = {'query': request.to_querystring(), 'version': version,
                  'client': 'deployed nova.scheduler.client.report singleton; nova.conf credentials',
                  'allocation_requests': candidates, 'provider_summaries': summaries,
                  'provider_found': isinstance(summaries, dict) and args.provider_id in summaries}
        (args.output / 'report.json').write_text(json.dumps(result, indent=2) + '\n')
        print(json.dumps({'provider_found': result['provider_found'], 'version': version}))
        return 0 if result['provider_found'] and candidates else 1
    except Exception as error:
        frames = [{'module': Path(f.filename).name, 'function': f.name, 'line': f.lineno}
                  for f in traceback.extract_tb(error.__traceback__)]
        (args.output / 'error.json').write_text(json.dumps({'error': type(error).__name__, 'frames': frames}) + '\n')
        print('Nova client probe failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())
