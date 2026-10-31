#!/usr/bin/env python3
"""Runtime-only notification scope for API/bytes benchmarks; originals untouched."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

DROPIN = Path('/run/systemd/system/devstack@g-api.service.d/90-tt-glance-benchmark.conf')
UNIT = 'devstack@g-api.service'


def noop_config(raw):
    """Change only the single existing notification driver in a private copy."""
    result, section, replacements = [], None, 0
    for line in raw.splitlines(keepends=True):
        header = re.match(r'^\s*\[([^]]+)\]', line)
        if header:
            section = header[1]
        if section == 'oslo_messaging_notifications' and re.match(r'^\s*driver\s*=', line):
            result.append('driver = noop\n'); replacements += 1
        else:
            result.append(line)
    if replacements != 1:
        raise ValueError('one explicit original notification driver required')
    return ''.join(result)


class Control:
    def __init__(self, root):
        self.root = Path(root)
        self.state = self.root / 'notification-policy.json'
        self.config = self.root / 'notification-config'

    def save(self, value):
        self.state.write_text(json.dumps(value, indent=2)+'\n'); self.state.chmod(0o600)

    def install(self):
        info = self.root.stat()
        if (self.root != self.root.resolve() or info.st_uid != os.getuid() or info.st_mode & 0o077
                or not re.fullmatch(r'tt-placement-[a-z0-9-]+', self.root.name) or DROPIN.exists() or self.state.exists()):
            raise ValueError('exclusive canonical private root and no prior override required')
        environment = subprocess.check_output(['systemctl','show','--value','-p','Environment',UNIT],text=True)
        if 'OS_GLANCE_CONFIG_DIR=' in environment:
            raise ValueError('pre-existing Glance config directory override; do not adopt')
        self.config.mkdir(mode=0o700,exist_ok=False)
        hashes = {}
        for source in Path('/etc/glance').iterdir():
            if source.is_file():
                raw = source.read_bytes(); hashes[source.name] = hashlib.sha256(raw).hexdigest()
                target = self.config/source.name; target.write_bytes(raw); target.chmod(0o600)
        path = self.config/'glance-api.conf'
        path.write_text(noop_config(path.read_text()))
        text = '[Service]\nEnvironment="OS_GLANCE_CONFIG_DIR='+str(self.config)+'"\n'
        local = self.root/'notification-dropin.conf'; local.write_text(text); local.chmod(0o600)
        state = {'policy':'noop','original_config_sha256':hashes,'dropin':str(DROPIN),
                 'dropin_sha256':hashlib.sha256(text.encode()).hexdigest(),'runtime_only':True,'restored':False}
        self.save(state)
        subprocess.run(['sudo','install','-D','-m','644',str(local),str(DROPIN)],check=True,stdout=subprocess.DEVNULL)
        subprocess.run(['sudo','systemctl','daemon-reload'],check=True)
        subprocess.run(['sudo','systemctl','restart',UNIT],check=True)
        return {k:v for k,v in state.items() if k!='dropin_sha256'}

    def restore(self):
        if not self.state.exists():
            return {'restored':True,'not_installed':True}
        state=json.loads(self.state.read_text())
        if state['restored']:
            return state
        if DROPIN.exists():
            if hashlib.sha256(DROPIN.read_bytes()).hexdigest()!=state['dropin_sha256']:
                raise ValueError('runtime override changed outside session')
            subprocess.run(['sudo','rm','--',str(DROPIN)],check=True)
        subprocess.run(['sudo','systemctl','daemon-reload'],check=True)
        subprocess.run(['sudo','systemctl','restart',UNIT],check=True)
        for name,digest in state['original_config_sha256'].items():
            if hashlib.sha256((Path('/etc/glance')/name).read_bytes()).hexdigest()!=digest:
                raise ValueError('original Glance configuration changed')
        environment = subprocess.check_output(['systemctl','show','--value','-p','Environment',UNIT],text=True)
        if 'OS_GLANCE_CONFIG_DIR=' in environment:
            raise ValueError('Glance runtime override not removed')
        state['restored']=True;self.save(state)
        return {'restored':True,'original_config_sha256':state['original_config_sha256'],'runtime_only':True}
