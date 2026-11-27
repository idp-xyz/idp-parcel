import { useCallback, useEffect, useState } from 'react';
import { Plus } from 'lucide-react';
import { Button, Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { statusToneBanner } from '../../components/status-tone';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { getSettingsOverview, runDueWaveRules, type SettingsOverviewBody } from './api';
import { useCloseOnHashChange } from './use-close-on-hash';
import { readFailureMessage } from './use-detail';
import { WaveRuleRunDialog } from './WaveRuleRunDialog';
import {
  dueWaveRulesNote,
  waveRulePrefill,
  waveRuleSummary,
  waveRuleTimesLabel,
  type DueWaveRulesBody,
  type WaveRuleRecord,
} from './wave-rule';
import { WmsBadge, WmsFilterSelect } from './wms-ui';

const moduleId = 'warehouse-waves';
const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const columns: ListColumn<WaveRuleRecord>[] = [
  { id: 'code', header: '规则码', className: mono, render: (row) => row.ruleCode },
  { id: 'name', header: '规则名', className: 'whitespace-nowrap', render: (row) => row.name },
  {
    id: 'state',
    header: '状态',
    render: (row) => <WmsBadge code={row.deactivated ? 'CANCELLED' : 'ACTIVE'} label={row.deactivated ? '已停用' : '在用'} />,
  },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'summary', header: '圈什么、怎么放', render: (row) => waveRuleSummary(row) },
  { id: 'revision', header: '版本', align: 'right', className: mono, render: (row) => `第 ${row.revision} 版` },
  { id: 'at', header: '登记时刻', className: mono, render: (row) => formatInstant(row.registeredAt) },
];

/**
 * 成波规则：各仓登记过的规则各自最新的一版。在这里登记、改、停用，从某一行直接按它成波；
 * 「放到点的规则」把今天已经到点、还没办过的时点办掉——平常由部署方的定时任务来调，这里给人手动补。
 */
export function WaveRuleList() {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const [answer, setAnswer] = useState<ApiResult<SettingsOverviewBody> | null>(null);
  const [tick, setTick] = useState(0);
  const [warehouse, setWarehouse] = useState('');
  const [running, setRunning] = useState<{ warehouse: string; rule: string } | null>(null);
  const [dueOpen, setDueOpen] = useState(false);
  const refresh = useCallback(() => setTick((value) => value + 1), []);

  useEffect(() => {
    let cancelled = false;
    void getSettingsOverview().then((loaded) => {
      if (!cancelled) setAnswer(loaded);
    });
    return () => {
      cancelled = true;
    };
  }, [tick]);

  const all = answer?.kind === 'outcome' ? (answer.body.waveRules ?? []) : [];
  const rows = warehouse === '' ? [...all] : all.filter((rule) => rule.warehouseCode === warehouse);
  const warehouseNames = new Map(drawer.lookups.warehouses.map((w) => [w.warehouseCode, w.name ?? '']));

  const inspectorOf = (row: WaveRuleRecord): InspectorContent => {
    const actions: InspectorAction[] = [];
    if (!row.deactivated && drawer.has('wave-release')) {
      actions.push({ label: '按这条规则成波', onRun: () => setRunning({ warehouse: row.warehouseCode, rule: row.ruleCode }) });
    }
    if (drawer.has('wave-rule')) {
      actions.push({ label: '改这条规则（登下一版）', onRun: () => drawer.open('wave-rule', waveRulePrefill(row)) });
      actions.push(
        row.deactivated
          ? { label: '重新启用', onRun: () => drawer.open('wave-rule', waveRulePrefill(row, { deactivated: false })) }
          : { label: '停用这条规则', onRun: () => drawer.open('wave-rule', waveRulePrefill(row, { deactivated: true })) },
      );
    }
    return {
      title: row.name,
      subtitle: `${row.warehouseCode} · ${row.ruleCode}`,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '圈什么、怎么放', value: waveRuleSummary(row) },
            { label: '每天的时点', value: waveRuleTimesLabel(row), mono: true },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: row.deactivated ? '已停用' : '在用' }] },
        { kind: 'actions', actions },
        { kind: 'audit', fields: [{ label: `第 ${row.revision} 版`, value: `${row.operator} · ${formatInstant(row.registeredAt)}`, mono: true }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<WaveRuleRecord>
        moduleId={moduleId}
        title="成波规则"
        description="各仓登记的成波规则：圈哪些已预占的出库单、一个波次多大、怎么拣、每天几点放。规则是你们自己定的，没登记的仓没有规则，放波次照旧由人勾单。"
        headerActions={
          <div className="flex flex-wrap gap-2">
            {drawer.has('wave-release') ? (
              <Button type="button" variant="outline" onClick={() => setDueOpen(true)}>
                放到点的规则
              </Button>
            ) : null}
            {drawer.has('wave-rule') ? (
              <Button
                type="button"
                className={primaryActionClass}
                onClick={() => drawer.open('wave-rule', warehouse ? { warehouse, revision: '1' } : { revision: '1' })}
              >
                <Plus className="mr-1 h-3.5 w-3.5" />
                登记成波规则
              </Button>
            ) : null}
          </div>
        }
        filters={
          <WmsFilterSelect
            label="仓库"
            value={warehouse}
            onChange={setWarehouse}
            options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: `${w.warehouseCode} · ${w.name}` }))}
          />
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => `${row.warehouseCode}/${row.ruleCode}`}
        inspector={inspectorOf}
        emptyRowsNote="这个仓没有登记成波规则"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-settings-overview',
          emptyTitle: '未配置：还没有登记成波规则',
          emptyDescription: '点右上角「登记成波规则」，按仓登一条：圈哪些单、一个波次最多几张、怎么拣、每天几点放。',
          narrowed: warehouse !== '',
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
      <WaveRuleRunDialog
        open={running !== null}
        onClose={() => setRunning(null)}
        onReleased={refresh}
        warehouseNames={warehouseNames}
        defaultWarehouse={running?.warehouse}
        defaultRule={running?.rule}
      />
      <DueWaveRulesDialog open={dueOpen} onClose={() => setDueOpen(false)} warehouse={warehouse} />
    </>
  );
}

/** 手动「放到点的规则」：说清它平常由谁来调、点了会怎样，点完把每个时点办的结果一句一句列出来。 */
function DueWaveRulesDialog({ open, onClose, warehouse }: { open: boolean; onClose: () => void; warehouse: string }) {
  useCloseOnHashChange(open, onClose);
  const [running, setRunning] = useState(false);
  const [answer, setAnswer] = useState<ApiResult<DueWaveRulesBody> | null>(null);
  useEffect(() => {
    if (open) setAnswer(null);
  }, [open]);

  const run = async () => {
    if (running) return;
    setRunning(true);
    const answered = await runDueWaveRules(warehouse || undefined);
    setRunning(false);
    setAnswer(answered);
  };

  const note = answer?.kind === 'outcome' ? dueWaveRulesNote(answer.body) : null;
  const failed = answer !== null && answer.kind !== 'outcome' ? (readFailureMessage(answer) ?? '没有送到') : null;

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <DialogContent className="flex max-h-[85vh] max-w-2xl flex-col">
        <DialogHeader>
          <DialogTitle>放到点的规则</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-4 overflow-y-auto">
          <p className="type-body text-idpxyz-textMuted">
            对登了「每天几点放」的规则，把今天已经过了、还没办过的时点各办一次：照规则圈单、放成波次。
            {warehouse ? `只看仓 ${warehouse} 的规则。` : '本租户各仓的规则都看。'}
          </p>
          <p className="type-body text-idpxyz-textMuted">
            这件事平常由部署方的定时任务按时来做，产品自己不计时。定时任务没配、或想马上补办时在这里点。同一个时点只办一次，点几次都不会放两遍；放出的波次记在你名下。
          </p>
          {note ? (
            <div role={note.tone === 'success' ? 'status' : 'alert'} className={`space-y-1 rounded border px-3 py-2 type-body ${statusToneBanner[note.tone]}`}>
              {note.lines.map((line) => (
                <p key={line}>{line}</p>
              ))}
            </div>
          ) : null}
          {failed ? (
            <p role="alert" className={`rounded border px-3 py-2 type-body ${statusToneBanner.warning}`}>
              {failed}。这一次有没有办成还不知道；再点一次不会放两遍。
            </p>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            关闭
          </Button>
          <Button type="button" className={primaryActionClass} disabled={running} onClick={() => void run()}>
            {running ? '正在办…' : note || failed ? '再办一次' : '放到点的规则'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
