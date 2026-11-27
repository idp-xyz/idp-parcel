import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { Plus, X } from 'lucide-react';
import { Button, Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, type WorkbenchColumn } from '../../templates';
import { cursorPageSummary } from '../../templates/cursor-pagination';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { statusToneBanner } from '../../components/status-tone';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueDefaultPageSize, cataloguePageSizes } from '../catalogue-query';
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
import { fitColumnWidth, nameOf } from './list-cells';
import { WmsBadge, WmsFilterSelect, WmsPagerBar, WmsTwoLine } from './wms-ui';

const moduleId = 'warehouse-waves';
const info = moduleInfoById[moduleId];
const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

function ruleKey(row: WaveRuleRecord): string {
  return `${row.warehouseCode}/${row.ruleCode}`;
}

/**
 * 成波规则：各仓登记过的规则各自最新的一版。在这里登记、改、停用，从某一行直接按它成波；
 * 「放到点的规则」把今天已经到点、还没办过的时点办掉——平常由部署方的定时任务来调，这里给人手动补。
 */
export function WaveRuleList() {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const [answer, setAnswer] = useState<ApiResult<SettingsOverviewBody> | null>(null);
  const [tick, setTick] = useState(0);
  const [warehouse, setWarehouse] = useTabViewState('warehouse-waves.rules.warehouse', '');
  const [page, setPage] = useTabViewState('warehouse-waves.rules.page', 1);
  const [pageSize, setPageSize] = useTabViewState('warehouse-waves.rules.pageSize', catalogueDefaultPageSize);
  const [openKey, setOpenKey] = useTabViewState<string | null>('warehouse-waves.rules.open', null);
  const [running, setRunning] = useState<{ warehouse: string; rule: string } | null>(null);
  const [dueOpen, setDueOpen] = useState(false);
  const refresh = useCallback(() => setTick((value) => value + 1), []);
  const size = (cataloguePageSizes as readonly number[]).includes(pageSize) ? pageSize : catalogueDefaultPageSize;

  useEffect(() => {
    let cancelled = false;
    void getSettingsOverview({ page, pageSize: size, warehouse: warehouse || undefined }).then((loaded) => {
      if (!cancelled) setAnswer(loaded);
    });
    return () => {
      cancelled = true;
    };
  }, [tick, page, size, warehouse]);

  const rows = answer?.kind === 'outcome' ? [...(answer.body.waveRules ?? [])] : [];
  const book = answer?.kind === 'outcome' ? answer.body.waveRuleBook : undefined;
  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((item) => [item.warehouseCode, item.name ?? ''])),
    [drawer.lookups.warehouses],
  );

  const columns: WorkbenchColumn<WaveRuleRecord>[] = [
    {
      key: 'name',
      header: '规则',
      width: fitColumnWidth,
      render: (row) => <WmsTwoLine primary={row.name} secondary={row.ruleCode} code title={`${row.name} · ${row.ruleCode}`} />,
    },
    {
      key: 'state',
      header: '状态',
      width: fitColumnWidth,
      render: (row) => <WmsBadge code={row.deactivated ? 'CANCELLED' : 'ACTIVE'} label={row.deactivated ? '已停用' : '在用'} />,
    },
    {
      key: 'warehouse',
      header: '仓',
      width: fitColumnWidth,
      minListWidth: 760,
      render: (row) => {
        const name = nameOf(warehouseNames, row.warehouseCode);
        return name ? <WmsTwoLine primary={name} secondary={row.warehouseCode} code title={`${name} · ${row.warehouseCode}`} /> : <span className="whitespace-nowrap font-mono">{row.warehouseCode}</span>;
      },
    },
    { key: 'summary', header: '圈什么、怎么放', render: (row) => <span className="type-body">{waveRuleSummary(row)}</span> },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) => (
        <RuleStep
          row={row}
          canRun={!row.deactivated && drawer.has('wave-release')}
          canEdit={drawer.has('wave-rule')}
          onRun={() => setRunning({ warehouse: row.warehouseCode, rule: row.ruleCode })}
          onEdit={() => drawer.open('wave-rule', waveRulePrefill(row, row.deactivated ? { deactivated: false } : undefined))}
        />
      ),
    },
  ];

  const viewState: TemplateViewState = catalogueViewState(answer, book?.total ?? rows.length, refresh, {
    module: info,
    endpoint: 'GET /warehouse-fulfillment-settings-overview',
    emptyTitle: '未配置：还没有登记成波规则',
    emptyDescription: '点右上角「登记成波规则」，按仓登一条：圈哪些单、一个波次最多几张、怎么拣、每天几点放。',
    narrowed: warehouse !== '',
  });

  return (
    <>
      <WorkbenchPageTemplate<WaveRuleRecord>
        title="成波规则"
        viewStateKey={`${moduleId}.rules`}
        subtitle="点一行在右边看这条规则。没登记的仓没有规则，放波次照旧由人勾单。"
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          <>
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
          </>
        }
        extraFilters={
          <WmsFilterSelect
            label="仓库"
            value={warehouse}
            onChange={(next) => {
              setWarehouse(next);
              setPage(1);
            }}
            options={drawer.lookups.warehouses.map((item) => ({
              value: item.warehouseCode,
              label: item.name ? `${item.name} · ${item.warehouseCode}` : item.warehouseCode,
            }))}
          />
        }
        columns={columns}
        rows={rows}
        rowKey={ruleKey}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        noMatchText="这个仓没有登记成波规则"
        onClearFilters={() => {
          setWarehouse('');
          setPage(1);
        }}
        renderDetail={(key) => {
          const row = rows.find((item) => ruleKey(item) === key);
          if (!row) return <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这条规则。</p>;
          return (
            <RuleDetail
              row={row}
              onClose={() => setOpenKey(null)}
              onRun={!row.deactivated && drawer.has('wave-release') ? () => setRunning({ warehouse: row.warehouseCode, rule: row.ruleCode }) : undefined}
              onEdit={drawer.has('wave-rule') ? () => drawer.open('wave-rule', waveRulePrefill(row)) : undefined}
              onRetire={
                drawer.has('wave-rule')
                  ? () => drawer.open('wave-rule', waveRulePrefill(row, { deactivated: !row.deactivated }))
                  : undefined
              }
            />
          );
        }}
        viewState={viewState}
        listFooter={
          book && viewState.kind === 'ready' ? (
            <WmsPagerBar
              summary={cursorPageSummary(book.page, book.pageSize, book.total)}
              canPrevious={page > 1}
              canNext={page * size < book.total}
              onPrevious={() => setPage(page - 1)}
              onNext={() => setPage(page + 1)}
              pageSize={{
                value: size,
                options: cataloguePageSizes,
                onChange: (next) => {
                  setPageSize(next);
                  setPage(1);
                },
              }}
            />
          ) : undefined
        }
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

function RuleStep({
  row,
  canRun,
  canEdit,
  onRun,
  onEdit,
}: {
  row: WaveRuleRecord;
  canRun: boolean;
  canEdit: boolean;
  onRun: () => void;
  onEdit: () => void;
}) {
  const run = !row.deactivated;
  const enabled = run ? canRun : canEdit;
  const label = run ? '按它成波' : '重新启用';
  if (!enabled) return <span className="whitespace-nowrap text-idpxyz-textMuted">{label}</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        if (run) onRun();
        else onEdit();
      }}
    >
      {label}
    </Button>
  );
}

function RuleDetail({
  row,
  onClose,
  onRun,
  onEdit,
  onRetire,
}: {
  row: WaveRuleRecord;
  onClose: () => void;
  onRun?: () => void;
  onEdit?: () => void;
  onRetire?: () => void;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-idpxyz-editor">
      <div className="flex shrink-0 items-start justify-between gap-2 border-b border-idpxyz-border px-4 py-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="type-section font-semibold text-idpxyz-textBright">{row.name}</span>
            <WmsBadge code={row.deactivated ? 'CANCELLED' : 'ACTIVE'} label={row.deactivated ? '已停用' : '在用'} />
          </div>
          <p className="mt-1 font-mono type-caption text-idpxyz-textMuted">{row.warehouseCode} · {row.ruleCode}</p>
        </div>
        <Button type="button" variant="ghost" size="icon" title="关闭详情" aria-label="关闭详情" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>
      <div className="min-h-0 flex-1 space-y-3 overflow-auto p-4 type-body">
        <p>{waveRuleSummary(row)}</p>
        <p className="font-mono type-caption text-idpxyz-textMuted">每天的时点 {waveRuleTimesLabel(row)}</p>
        <p className="font-mono type-caption text-idpxyz-textMuted">第 {row.revision} 版 · {row.operator} · {formatInstant(row.registeredAt)}</p>
        <div className="flex flex-wrap gap-2">
          {onRun ? (
            <Button type="button" size="sm" variant="outline" onClick={onRun}>
              按它成波
            </Button>
          ) : null}
          {onEdit ? (
            <Button type="button" size="sm" variant="outline" onClick={onEdit}>
              改下一版
            </Button>
          ) : null}
          {onRetire ? (
            <Button type="button" size="sm" variant="outline" onClick={onRetire}>
              {row.deactivated ? '重新启用' : '停用'}
            </Button>
          ) : null}
        </div>
      </div>
    </div>
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
