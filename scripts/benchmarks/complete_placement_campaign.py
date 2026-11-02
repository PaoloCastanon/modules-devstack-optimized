#!/usr/bin/env python3
"""Durable completion of an authorized long campaign in an isolated worktree.

The config contains public paths/PR number, never credentials. Publishing and VM
shutdown are explicit independent opt-ins. No merge, force push or original SQL.
"""
import argparse
import datetime
import gzip
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import time
from zoneinfo import ZoneInfo

from analyze_placement import analyze
from publish_placement_results import render


def save(path, value):
    temporary = path.with_suffix('.next')
    temporary.write_text(json.dumps(value, indent=2) + '\n')
    temporary.replace(path)


def cmd(values, **kwargs):
    return subprocess.run(values, check=True, **kwargs)


def compact(root):
    for raw in root.rglob('*.jsonl'):
        target = Path(str(raw) + '.gz')
        if target.exists():
            raise ValueError('refuse to replace raw observations')
        with raw.open('rb') as source, gzip.open(target, 'wb') as output:
            shutil.copyfileobj(source, output)
        raw.unlink()
    normalization = {}
    for log in root.rglob('*.log'):
        original = log.read_bytes()
        cleaned = ('\n'.join(line.rstrip() for line in original.decode('utf-8').rstrip().splitlines()) + '\n').encode()
        if original == cleaned or not original:
            continue
        # Retain exact bytes before making SDK warning logs diff-check friendly.
        archive = Path(str(log) + '.raw.gz')
        with archive.open('xb') as stream:
            stream.write(gzip.compress(original))
        log.write_bytes(cleaned)
        normalization[str(log.relative_to(root))] = {'raw_sha256': hashlib.sha256(original).hexdigest(),
                                                    'saved_sha256': hashlib.sha256(cleaned).hexdigest(),
                                                    'raw_archive': str(archive.relative_to(root)),
                                                    'change': 'trailing whitespace and blank EOF only'}
    if normalization:
        save(root / 'text-normalization.json', normalization)


def safely_shutdown(manifest, output):
    report = manifest.get('cleanup_report', {})
    required = ['original_unchanged', 'runtime_removed', 'identity_removed', 'private_root_removed']
    units = ['devstack@placement-api.service', 'devstack@n-cpu.service', 'devstack@n-sch.service', 'devstack@keystone.service', 'devstack@g-api.service']
    if not manifest.get('cleanup') or not all(report.get(k) is True for k in required) or not all(report.get('original_services_active', {}).get(unit) is True for unit in units):
        raise ValueError('restoration not proved; leave lab available for repair')
    cmd(['virsh', '-c', 'qemu:///system', 'shutdown', 'tt-openstack-lab'], stdout=subprocess.DEVNULL)
    for _ in range(60):
        state = subprocess.check_output(['virsh', '-c', 'qemu:///system', 'domstate', 'tt-openstack-lab'], text=True).strip()
        if state == 'shut off':
            save(output, {'domain': 'tt-openstack-lab', 'state': state, 'forced': False,
                          'timestamp_utc': datetime.datetime.now(datetime.timezone.utc).isoformat()})
            return
        time.sleep(2)
    raise ValueError('graceful VM shutdown not completed')


def commit(paths, message):
    cmd(['git', 'add', '--', *map(str, paths)])
    cmd(['git', 'diff', '--cached', '--check'])
    cmd(['git', 'commit', '-m', message])


def run(args):
    if not args.allow_mutations or not args.allow_publish or not args.allow_vm_shutdown:
        raise ValueError('explicit campaign/publish/shutdown opt-ins required')
    config = json.loads(args.config.read_text())
    worktree = Path(config['worktree']).resolve()
    if worktree != Path.cwd().resolve():
        raise ValueError('run only in designated isolated worktree')
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    if head != config['frozen_head'] or subprocess.check_output(['git', 'status', '--porcelain'], text=True).strip():
        raise ValueError('frozen clean worktree required')
    status_file = Path(config['status_file'])
    status = {'stage': 'running', 'started_local': datetime.datetime.now(ZoneInfo('America/Santiago')).isoformat(),
              'required_runs': 240, 'completed_runs': 0, 'accepted': False, 'pr': config['pr_url']}
    save(status_file, status)
    output = Path(config['output'])
    evidence = worktree / config['evidence']
    log = Path(config['log'])
    try:
        with log.open('x') as stream:
            child = subprocess.Popen([sys.executable, 'scripts/benchmarks/placement_campaign.py', *config['campaign_args']], stdout=stream, stderr=subprocess.STDOUT)
            while child.poll() is None:
                if (output / 'manifest.json').exists():
                    try:
                        current = json.loads((output / 'manifest.json').read_text())
                        status['completed_runs'] = len(current['runs'])
                        status['last_run'] = current['runs'][-1] if current['runs'] else None
                        status['updated_utc'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
                        save(status_file, status)
                    except json.JSONDecodeError:
                        pass  # Manifest write can race this read; retry next poll.
                time.sleep(30)
        manifest = json.loads((output / 'manifest.json').read_text())
        status.update(stage='verifying', campaign_exit=child.returncode, completed_runs=len(manifest['runs']))
        save(status_file, status)
        evidence.mkdir(parents=True, exist_ok=False)
        shutil.copytree(output, evidence / 'official')
        shutil.copy(log, evidence / 'campaign.log')
        compact(evidence)
        safely_shutdown(manifest, evidence / 'vm-final-state.json')
        status['vm'] = 'shut off'
        save(status_file, status)
        if child.returncode or not manifest['valid']:
            raise ValueError('campaign invalid; raw evidence retained, no acceptance published')
        summary = analyze(evidence / 'official')
        save(evidence / 'summary.json', summary)
        if not summary['b17_accepted'] or not summary['b18_accepted']:
            raise ValueError('full B17/B18 analysis gates not passed')
        render(summary, evidence / 'publication')
        with (evidence / 'make-check.log').open('x') as stream:
            cmd(['make', 'check'], stdout=stream, stderr=subprocess.STDOUT)
        # An isolated worktree prevents unrelated user edits from entering these
        # commits. Recheck HEAD before staging the explicitly named results.
        if subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip() != config['frozen_head']:
            raise ValueError('worktree HEAD changed during campaign')
        if subprocess.check_output(['git', 'diff', '--name-only'], text=True).strip() or subprocess.check_output(['git', 'diff', '--cached', '--name-only'], text=True).strip():
            raise ValueError('tracked worktree or index changed during campaign')
        sources = {str(p.relative_to(worktree)): hashlib.sha256(p.read_bytes()).hexdigest() for p in (worktree / 'scripts/benchmarks').glob('*.py')}
        save(evidence / 'source-files.json', sources)
        date = datetime.datetime.now(ZoneInfo('America/Santiago')).strftime('%Y-%m-%d')
        base_link = '../../' + config['evidence']
        # Generated tables already carry actual medians/CIs and deployment limits.
        b17 = worktree / 'docs/benchmarks/b17-results.md'
        b18 = worktree / 'docs/benchmarks/b18-results.md'
        b17.write_text((evidence / 'publication/b17-results.md').read_text() + f'\nEvidence: [full summary]({base_link}/summary.json), [raw campaign]({base_link}/official/manifest.json).\n')
        build_log = worktree / 'harness/build-log.md'
        with build_log.open('a') as stream:
            stream.write(f'\n### {date} — B17 medición oficial Placement completada\n\n'
                         '- 240/240 ventanas válidas: seis cargas, concurrencias 1/4/8/16, cinco pares independientes; warm-up30s/carga120s, protocolo B16 sin recortes.\n'
                         '- Requests crudas, status/errores/generaciones, goodput, p50/p95/p99 y bootstrap de pares conservados. Semilla equivalente, owner SQL reutilizado, limpieza y original restaurado.\n'
                         '- make check PASS; VM shut off. Informe docs/benchmarks/b17-results.md. Excel y S2 intactos; no inferencia causal exclusiva del lenguaje.\n')
        commit([evidence, b17, build_log], 'feat(benchmarks): complete B17 full 240-run Placement latency and throughput campaign')
        b18.write_text((evidence / 'publication/b18-results.md').read_text() + f'\nEvidence: [aligned CPU/PSS and controls]({base_link}/summary.json), [restoration and VM]({base_link}/vm-final-state.json).\n')
        with build_log.open('a') as stream:
            stream.write(f'\n### {date} — B18 consumo CPU/PSS correlacionado completado\n\n'
                         '- Las mismas 240 ventanas de B17: cgroups completos, PID/start_ticks estables, ticks CPU y PSS a1Hz; reposo, carga, máximo observado y RSS secundario.\n'
                         '- CPU/op usa requests completadas en intervalo interior común; relojes y overhead del recolector registrados. MySQL/Keystone/Apache/Nova/Glance/generador separados.\n'
                         '- Cinco pares por celda e intervalos bootstrap; datos crudos y resumen reproducible. Sin heap/GC expuesto; diferencias de auth/cache/pools/DB declaradas.\n'
                         '- make check PASS. Restauración, cleanup y VM shut off comprobados; B17/B18 cerrados técnicamente para este subconjunto de Placement.\n')
        plans = worktree / 'PLANS.md'
        text = plans.read_text()
        begin, end = '<!-- placement-b17-b18-status:start -->', '<!-- placement-b17-b18-status:end -->'
        if text.count(begin) != 1 or text.count(end) != 1:
            raise ValueError('expected planning status markers missing')
        a, b = text.index(begin), text.index(end) + len(end)
        plans.write_text(text[:a] + begin + f'\n**B17/B18 completados técnicamente, {date}:** '
                         '240/240 ventanas oficiales válidas y análisis CPU/PSS correlacionado aprobado. '
                         'Ver [B17](docs/benchmarks/b17-results.md) y [B18](docs/benchmarks/b18-results.md). '
                         'Original restaurado, limpieza comprobada y VM apagada. Excel/S2 conservan su aceptación histórica.\n' + end + text[b:])
        implementation = worktree / 'docs/placement/implementation-plan.md'
        with implementation.open('a') as stream:
            stream.write(f'\nActualización {date}: B17/B18 cerrados técnicamente con [latencia/throughput](../benchmarks/b17-results.md) '
                         'y [CPU/PSS](../benchmarks/b18-results.md), 240 ventanas oficiales válidas, original restaurado y VM apagada.\n')
        save(evidence / 'b18-acceptance.json', {'b17_accepted': True, 'b18_accepted': True, 'runs': 240, 'vm': 'shut off', 'make_check': 'PASS'})
        commit([b18, build_log, plans, implementation, evidence / 'b18-acceptance.json'], 'feat(benchmarks): complete B18 aligned Placement CPU and PSS evaluation')
        cmd(['git', 'push', 'origin', 'HEAD'])
        body = Path(config['pr_body_file'])
        body.write_text('Completa B17/B18 para el subconjunto Placement con las 240 ventanas oficiales del protocolo B16, cinco pares por carga/concurrencia. '
                        'Incluye requests crudas, p50/p95/p99, goodput, CPU/PSS de todos los workers, controles y análisis bootstrap reproducible. '
                        'Workers/cachés de auth/pools/configuración MySQL distintos se declaran: no se atribuye el resultado sólo al lenguaje.\n\n'
                        'Validación: 240/240 corridas válidas; make check PASS; restauración y limpieza verificadas; VM apagada. '
                        'Pilotos y fallos de preparación conservados por separado. Excel/S2 y Glance original se preservan.\n')
        cmd(['gh', 'pr', 'edit', str(config['pr_number']), '--title', 'Placement B17/B18: evaluación completa de latencia, throughput y CPU/PSS', '--body-file', str(body)])
        ci_passed = False
        for _ in range(60):
            checks = subprocess.run(['gh', 'pr', 'checks', str(config['pr_number']), '--json', 'name,bucket,link'], capture_output=True, text=True)
            try:
                values = json.loads(checks.stdout)
            except ValueError:
                values = []
            if values and all(v['bucket'] == 'pass' for v in values):
                ci_passed = True
                break
            if any(v['bucket'] in ('fail', 'cancel') for v in values):
                break
            time.sleep(20)
        if not ci_passed:
            raise ValueError('CI not passing; results pushed but PR stays draft')
        cmd(['gh', 'pr', 'ready', str(config['pr_number'])])
        status.update(stage='complete', accepted=True, b17=True, b18=True, ci='PASS',
                      head=subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
                      finished_local=datetime.datetime.now(ZoneInfo('America/Santiago')).isoformat())
        save(status_file, status)
        return 0
    except Exception as error:
        status.update(stage='needs_attention', error=type(error).__name__,
                      reason=str(error) if isinstance(error, ValueError) else 'see execution log',
                      finished_local=datetime.datetime.now(ZoneInfo('America/Santiago')).isoformat())
        save(status_file, status)
        return 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', type=Path, required=True)
    parser.add_argument('--allow-mutations', action='store_true')
    parser.add_argument('--allow-publish', action='store_true')
    parser.add_argument('--allow-vm-shutdown', action='store_true')
    args = parser.parse_args()
    sys.exit(run(args))


if __name__ == '__main__':
    main()
