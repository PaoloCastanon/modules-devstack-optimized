#!/usr/bin/env python3
"""Manage only a disposable tt-placement identity using the VM's admin openrc.

Run with the deployed Keystone SDK. Private state and application secrets never
leave --private-root. Existing Nova credentials are used only to issue a token.
"""
import argparse
import datetime
import json
import os
from pathlib import Path
import secrets
import shutil
import sys


def run(args):
    if not args.allow_mutations:
        raise ValueError('explicit mutation opt-in required')
    root = args.private_root
    if not root.is_absolute() or not root.name.startswith('tt-placement-') or root.is_symlink():
        raise ValueError('absolute owned tt-placement private directory required')
    from keystoneauth1 import session
    from keystoneauth1.identity import v3
    from keystoneclient.v3 import client
    from keystoneclient import exceptions
    if args.action == 'prepare':
        root.mkdir(mode=0o700, parents=False, exist_ok=False)
        state = {'apps': [], 'name': 'tt-placement-' + secrets.token_hex(8)}
    else:
        if root.stat().st_mode & 0o077 or root.stat().st_uid != os.getuid():
            raise ValueError('private directory ownership/mode mismatch')
        state = json.loads((root / 'state.json').read_text())

    def save(name, value):
        path = root / name
        path.write_text(json.dumps(value) if not isinstance(value, str) else value)
        path.chmod(0o600)

    def checkpoint():
        save('state.json', state)

    checkpoint()
    auth_url = os.environ['OS_AUTH_URL'].rstrip('/')
    if not auth_url.endswith('/v3'):
        auth_url += '/v3'
    plugin = v3.Password(auth_url=auth_url, username=os.environ['OS_USERNAME'],
                         password=os.environ['OS_PASSWORD'], project_name=os.environ['OS_PROJECT_NAME'],
                         user_domain_name=os.environ.get('OS_USER_DOMAIN_NAME', 'Default'),
                         project_domain_name=os.environ.get('OS_PROJECT_DOMAIN_NAME', 'Default'))
    admin = client.Client(session=session.Session(auth=plugin, timeout=15))

    if args.action == 'prepare':
        domain = admin.domains.list(name='Default')[0].id
        state['project'] = admin.projects.create(state['name'], domain=domain).id
        checkpoint()
        state['password'] = secrets.token_urlsafe(32)
        checkpoint()
        state['user'] = admin.users.create(state['name'], domain=domain, password=state['password'], default_project=state['project']).id
        checkpoint()
        roles = {role.name: role.id for role in admin.roles.list()}
        for role in ['admin', 'service', 'reader']:
            admin.roles.grant(roles[role], user=state['user'], project=state['project'])
        user = client.Client(session=session.Session(auth=v3.Password(auth_url=auth_url, user_id=state['user'], password=state['password'], project_id=state['project']), timeout=15))
        for role in ['admin', 'service', 'reader']:
            expiry = datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=8)
            app = user.application_credentials.create(name='placement-lab-' + role, user=state['user'], roles=[{'name': role}], expires_at=expiry)
            credential = {'application_credential_id': app.id, 'application_credential_secret': app.secret}
            state['apps'].append({'id': app.id, 'role': role})
            checkpoint()
            save(role + '.json', credential)
            if role == 'service':
                save('validator.json', credential)

    if args.action in ('prepare', 'refresh'):
        metadata = {'project': state['project'], 'user': state['user'], 'tokens': {}}
        for role in ['admin', 'service', 'reader']:
            credential = json.loads((root / (role + '.json')).read_text())
            plugin = v3.ApplicationCredential(auth_url=auth_url, **credential)
            scoped = session.Session(auth=plugin, timeout=15)
            token = scoped.get_token()
            access = plugin.get_access(scoped)
            if role not in access.role_names or role != 'admin' and 'admin' in access.role_names:
                raise ValueError('unexpected role implication')
            save(role + '.token', token + '\n')
            metadata['tokens'][role] = {'expires_at': access.expires.isoformat(), 'roles': access.role_names}
        save('metadata.json', metadata)
        print(json.dumps(metadata))
        return 0

    # Verify ownership against Keystone before deleting any identity.
    for key, manager in [('user', admin.users), ('project', admin.projects)]:
        if state.get(key):
            try:
                value = manager.get(state[key])
                if value.name != state['name'] or not value.name.startswith('tt-placement-'):
                    raise ValueError('identity ownership mismatch')
            except exceptions.NotFound:
                pass
    # Deleting this owned user removes its application credentials and tokens.
    for key, manager in [('user', admin.users), ('project', admin.projects)]:
        if state.get(key):
            try:
                manager.delete(state[key])
            except exceptions.NotFound:
                pass
    absent = {}
    for key, manager in [('user', admin.users), ('project', admin.projects)]:
        if not state.get(key):
            absent[key] = True
            continue
        try:
            manager.get(state[key])
            absent[key] = False
        except exceptions.NotFound:
            absent[key] = True
    if not all(absent.values()):
        raise RuntimeError('identity cleanup incomplete')
    shutil.rmtree(root)
    print(json.dumps({'cleanup_verified': True, 'absent': absent, 'private_root_removed': True}))
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'refresh', 'cleanup'])
    parser.add_argument('--private-root', type=Path, required=True)
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    try:
        return run(args)
    except Exception as error:
        print('lab identity failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())
