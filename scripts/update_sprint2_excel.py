#!/usr/bin/env python3
"""Apply evidence-backed Sprint 2 status to a copy of the planning book."""
from pathlib import Path
from openpyxl import load_workbook
from openpyxl.workbook.properties import CalcProperties

root = Path(__file__).resolve().parents[1]
source = root / 'Planificacion_Trabajo_Titulo_OpenStack_Placement_Glance.xlsx'
backup = root / 'Planificacion_Trabajo_Titulo_OpenStack_Placement_Glance.pre-sprint2.xlsx'
target = root / 'Planificacion_Trabajo_Titulo_OpenStack_Placement_Glance_Sprint2.xlsx'
assert source.exists() and backup.exists()
assert source.read_bytes() == backup.read_bytes(), 'Original/backup differ'
book = load_workbook(source)
assert book.sheetnames == ['Resumen','Objetivos_SMART','Requerimientos','Backlog','Trazabilidad','Sprints','Dashboard','Listas']
backlog = book['Backlog']
data = {
    'B01': ('Bloqueado','evidence/sprint2/environment/host-audit.txt','Host CachyOS no dedicado; se requiere VM Ubuntu 24.04.'),
    'B02': ('Bloqueado','infra/devstack/setup-devstack.sh; docs/sprint2/BLOCKERS.md','DevStack preparado, stack.sh no ejecutado en esta estación.'),
    'B03': ('Bloqueado','infra/devstack/smoke-test.sh; docs/sprint2/BLOCKERS.md','Suite preparada; sin DevStack para ejecutar ni aceptar casos.'),
    'B04': ('En progreso','docs/sprint2/placement-architecture.md; evidence/sprint2/placement/upstream-source-audit.txt','Arquitectura upstream inspeccionada; falta servicio real.'),
    'B05': ('En progreso','docs/sprint2/placement-api-scope.md; evidence/sprint2/placement/upstream-source-audit.txt','Inventario preliminar; faltan microversiones probadas y baseline.'),
    'B06': ('Bloqueado','docs/sprint2/nova-placement-flow.md; docs/sprint2/BLOCKERS.md','Código Nova inspeccionado; tráfico real no observado.'),
    'B19': ('En progreso','docs/sprint2/glance-api-scope.md; docs/sprint2/glance-architecture.md; evidence/sprint2/glance/upstream-source-audit.txt','Diseño upstream disponible; falta backend y flujo reales.'),
}
changed = set()
for row in range(4, backlog.max_row + 1):
    key = backlog.cell(row, 1).value
    if key in data:
        state, evidence, observation = data[key]
        assert backlog.cell(row, 11).value == 'S2'
        backlog.cell(row, 12, state)
        backlog.cell(row, 16, evidence)
        backlog.cell(row, 17, observation)
        changed.add(key)
assert changed == set(data), changed
objectives = book['Objetivos_SMART']
assert objectives['A4'].value == 'OE1' and objectives['A5'].value == 'OE2'
objectives['K4'] = 'En progreso'
objectives['L4'] = 'Diseño upstream documentado; faltan trazas y baseline reales.'
objectives['K5'] = 'Bloqueado'
objectives['L5'] = 'No hay DevStack instalado; host no dedicado. Ver docs/sprint2/BLOCKERS.md.'
sprints = book['Sprints']
assert sprints['A5'].value == 'S2'
sprints['J5'] = 'Bloqueado'
sprints['K5'] = '0/33 story points aceptados; host no dedicado, sin DevStack ni baseline.'
book.calculation = CalcProperties(fullCalcOnLoad=True)
book.save(target)
check = load_workbook(target)
assert check.sheetnames == book.sheetnames
assert sum(len(s._charts) for s in check) == 5
assert sum(c.data_type == 'f' for s in check for row in s for c in row) == sum(c.data_type == 'f' for s in book for row in s for c in row)
assert sum(len(s.data_validations.dataValidation) for s in check) == sum(len(s.data_validations.dataValidation) for s in book)
print(f'Updated {target.name}: {", ".join(sorted(changed))}; formulas/charts/validations preserved')
