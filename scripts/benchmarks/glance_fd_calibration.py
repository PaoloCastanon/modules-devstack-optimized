#!/usr/bin/env python3
"""Controlled FD diagnostic, not performance evidence: original vs runtime noop."""
import argparse
import json
import logging
from pathlib import Path
import re
import subprocess
import sys
import uuid

from glance_http import Client, payload
from glance_notifications import Control
from glance_preflight import Session
import glance_switch
from placement_suite import exclusive_run, require, save_json


def fd_counts():
    code = """import pathlib,json,os
p=pathlib.Path('/sys/fs/cgroup/system.slice/system-devstack.slice/devstack@g-api.service/cgroup.procs')
rows=[]
for pid in p.read_text().split():
 entries=list(pathlib.Path('/proc',pid,'fd').iterdir()); sockets=0; cache=0
 for entry in entries:
  try:target=os.readlink(entry)
  except FileNotFoundError:continue
  sockets+=target.startswith('socket:');cache+='/glance/cache/' in target
 rows.append({'pid':int(pid),'fds':len(entries),'sockets':sockets,'cache_files':cache})
print(json.dumps(rows))
"""
    return json.loads(subprocess.check_output(['sudo',sys.executable,'-c',code],text=True))


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ['work-root','output','binary']:
        parser.add_argument('--'+name,type=Path,required=True)
    parser.add_argument('--url',default='http://192.168.122.10/image')
    parser.add_argument('--allow-mutations',action='store_true')
    args=parser.parse_args()
    require(args.allow_mutations and args.work_root==args.work_root.resolve() and re.fullmatch(r'tt-placement-[a-z0-9-]+',args.work_root.name),'owned new diagnostic and opt-in')
    logging.disable(logging.CRITICAL)
    args.output.mkdir(parents=True,exist_ok=False)
    report={'valid':False,'cleanup':False,'scope':'diagnostic only, VM-local client; not performance acceptance','modes':{}}
    with exclusive_run(Path('/run/lock/tt-placement-integration.lock')):
        args.work_root.mkdir(mode=0o700,exist_ok=False)
        session=Session(args);control=Control(args.work_root)
        try:
            session.prepare()
            size=1024**2;data=args.work_root/'payload.bin';expected=payload(data,size)
            ident=str(uuid.uuid4());base='/v2/images/'+ident;name='tt-glance-fd-calibration'
            session.request(args.url,'GET',base,404)
            session.pending[('python-fd',ident)]={'url':args.url,'name':name};session.python_ids.append(ident)
            session.request(args.url,'POST','/v2/images',201,body={'id':ident,'name':name,'tags':[session.tag],
                            'visibility':'private','disk_format':'raw','container_format':'bare'})
            session.request(args.url,'PUT',base+'/file',204,upload=data,content_type='application/octet-stream')
            for mode,count in [('original',1500),('noop',6000)]:
                if mode=='noop':
                    report['policy']=control.install();glance_switch.wait_auth(args.url)
                probes=[{'requests':0,'processes':fd_counts()}]
                client=Client(args.url,session.token)
                success,errors=0,[]
                try:
                    for i in range(count):
                        sample,_=client.request('GET',base+'/file',expected_payload=expected)
                        if sample['status']!=200 or sample['error_kind']:
                            errors.append({'request':i+1,'status':sample['status'],'bytes':sample['response_bytes'],'error_kind':sample['error_kind']});break
                        success+=1
                        if (i+1)%250==0:probes.append({'requests':i+1,'processes':fd_counts()})
                finally:client.close()
                report['modes'][mode]={'correct':success,'planned':count,'errors':errors,'probes':probes}
                save_json(args.output/'progress.json',report)
                require(not errors and success==count,'diagnostic requests all complete')
            original=report['modes']['original']['probes'];noop=report['modes']['noop']['probes']
            original_growth=sum(p['fds'] for p in original[-1]['processes'])-sum(p['fds'] for p in original[0]['processes'])
            noop_growth=sum(p['fds'] for p in noop[-1]['processes'])-sum(p['fds'] for p in noop[0]['processes'])
            report.update(original_fd_growth=original_growth,noop_fd_growth=noop_growth)
            require(original_growth>1000 and noop_growth<100,'notification control removes demonstrated descriptor accumulation')
            report['valid']=True
        except Exception as error:
            report.update(error=type(error).__name__,reason=str(error) if isinstance(error,AssertionError) else 'private diagnostic retained')
        finally:
            try:
                report['notification_restoration']=control.restore();glance_switch.wait_auth(args.url)
                report['cleanup_report']=session.cleanup();report['cleanup']=True
            except Exception as error:
                report['cleanup_error']=type(error).__name__
            report['valid']=report['valid'] and report['cleanup']
            save_json(args.output/'report.json',report)
    print(json.dumps({k:report.get(k) for k in ['valid','cleanup','original_fd_growth','noop_fd_growth','error']}))
    return 0 if report['valid'] else 1


if __name__=='__main__':
    sys.exit(main())
