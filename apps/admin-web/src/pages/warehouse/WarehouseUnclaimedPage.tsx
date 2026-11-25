import { useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent, InspectorField } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { listUnclaimedArrivals, type DisposalRuleRecord, type UnclaimedArrivalRecord } from './api';
import { DocumentLink } from './document-page';
import { claimPrefill, unclaimedNextStep } from './unclaimed-next-step';
import { UnclaimedDocument } from './UnclaimedDocument';
import { openDocument, useDocumentParts } from './use-document';
import { useDisposalRules } from './use-disposal-rules';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const moduleId = 'warehouse-unclaimed';
const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 签的先后照一件到货往下走的先后。待认领与已认领是要人动手的两队：认领被撤销后到货退回待认领，两个数都会往回走，所以都数。
// 已处置只增不减，数出来的不是待办，不数；「全部」放最后，页面打开就落在待认领上。
const queueStatuses = ['UNCLAIMED', 'CLAIMED'] as const;
const settledStatuses = ['DISPOSED'] as const;
const kindWords = ['INBOUND', 'RETURN'] as const;

const newestFirst = '-receivedAt';
const oldestFirst = 'receivedAt';

/** 待认领到货：列表与每一件到货各占自己的标签，同一个模块按地址里有没有单据键分。 */
export function WarehouseUnclaimedPage() {
  const parts = useDocumentParts();
  return parts ? <UnclaimedDocument arrivalId={parts[0]} /> : <UnclaimedList />;
}

function UnclaimedList() {
  const drawer = useActionDrawer(receivingActionSpecs);
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<string>('UNCLAIMED');
  const [warehouse, setWarehouse] = useState('');
  const [kind, setKind] = useState('');
  const [sort, setSort] = useState<string | null>(null);
  const [refreshes, setRefreshes] = useState(0);
  const rules = useDisposalRules(refreshes);

  const settled = useSettledKeyword(keyword);
  const baseFilters: Record<string, string[]> = {};
  if (warehouse) baseFilters.warehouseCode = [warehouse];
  if (kind) baseFilters.kind = [kind];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  // 选了待认领或已认领这样的队，按登记先后排，放得最久的在最上面；看全部或已处置的，最新的在上面。人改过排序就照他的。
  // 读口可排的是登记时刻（receivedAt），认领期限却是从到货发生时刻起算：线下先记、后同步上来的到货两者会差开，差得不大。
  const inQueue = (queueStatuses as readonly string[]).includes(status);
  const effectiveSort = sort ?? (inQueue ? oldestFirst : newestFirst);
  const conditions: CatalogueConditions = {
    ...base,
    sort: effectiveSort,
    filters: status === 'ALL' ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listUnclaimedArrivals);
  const rows = answer?.kind === 'outcome' ? answer.body.arrivals : [];
  const counts = useStatusCounts(listUnclaimedArrivals, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const now = new Date();

  const columns: ListColumn<UnclaimedArrivalRecord>[] = [
    { id: 'id', header: '到货号', className: 'whitespace-nowrap', render: (row) => <DocumentLink moduleId={moduleId} parts={[row.arrivalId]} /> },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
    { id: 'warehouse', header: '仓库', className: mono, render: (row) => row.warehouseCode },
    { id: 'kind', header: '种类', className: 'whitespace-nowrap', render: (row) => labelOf(row.kind) },
    { id: 'obs', header: '现场观察', className: 'max-w-[18rem] truncate', render: (row) => row.observation || '—' },
    {
      id: 'age',
      header: '已放',
      align: 'right',
      className: 'whitespace-nowrap',
      // 已处置的不在等谁，不写放了多久；认领期限也是从到货发生时刻起算，所以这里同样从发生时刻起算。
      render: (row) => (row.status === 'DISPOSED' ? formatInstant(row.receivedAt) : elapsedLabel(row.occurredAt, now) || '—'),
    },
    {
      id: 'next',
      header: '下一步',
      className: 'whitespace-nowrap',
      render: (row) => <NextStepCell row={row} rule={rules?.get(row.warehouseCode)} now={now} drawer={drawer} />,
    },
  ];

  const inspectorOf = (row: UnclaimedArrivalRecord): InspectorContent => {
    const fields: InspectorField[] = [
      { label: '事实标识', value: row.factId, mono: true },
      { label: '仓库 / 来源', value: `${row.warehouseCode} · ${labelOf(row.origin)}`, mono: true },
      { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
      { label: '观察', value: row.observation || '—' },
      { label: '货主 / 预报线索', value: `${row.ownerClue || '—'} / ${row.preAdviceClue || '—'}`, mono: true },
      { label: '存放处', value: row.storageLocation || '—', mono: true },
    ];
    if (row.disposition) {
      fields.push(
        { label: '处置方式与规则', value: `${labelOf(row.disposition.method)}（${row.disposition.ruleWarehouseCode} r${row.disposition.ruleRevision}）` },
        { label: '处置时刻', value: formatInstant(row.disposition.disposedAt), mono: true },
      );
    }

    const actions: InspectorAction[] = [{ label: '打开待认领到货', onRun: () => openDocument(moduleId, [row.arrivalId]) }];
    if (row.status === 'UNCLAIMED') {
      if (drawer.has('unclaimed-claim')) {
        actions.push({ label: '认领这件到货', onRun: () => drawer.open('unclaimed-claim', claimPrefill(row)) });
      }
      if (drawer.has('unclaimed-dispose')) {
        actions.push({ label: '按规则处置', onRun: () => drawer.open('unclaimed-dispose', { arrivalId: row.arrivalId }) });
      }
    } else if (row.status === 'CLAIMED' && drawer.has('unclaimed-revoke')) {
      actions.push(
        (row.postingReferences?.length ?? 0) > 0
          ? { label: '撤销认领', disabledReason: '这一认领已被收货入账引用，不能撤销' }
          : { label: '撤销认领', onRun: () => drawer.open('unclaimed-revoke', { arrivalId: row.arrivalId }) },
      );
    }

    return {
      title: row.arrivalId,
      subtitle: labelOf(row.status),
      sections: [
        { kind: 'summary', fields },
        { kind: 'status', items: [{ label: '状态', word: labelOf(row.status) }] },
        { kind: 'actions', actions },
        { kind: 'audit', fields: [{ label: '操作者 / 设备', value: `${row.operator} · ${row.device}`, mono: true }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<UnclaimedArrivalRecord>
        moduleId={moduleId}
        title={info.title}
        description="无主到货与逾期到达。点状态看那一队，数量是当前筛选下还有几件。点到货号打开待认领到货；放得最久的排在前面，下一步一列写明是认领、处置还是去作业台入账。"
        headerActions={
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-settings'; }}>
              管理处置规则
            </Button>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
              去作业台登记到货
            </Button>
          </div>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜到货号、仓或观察描述' }}
        sort={{
          value: effectiveSort,
          onChange: setSort,
          options: [
            { value: newestFirst, label: '最新登记的在前' },
            { value: oldestFirst, label: '放得最久的在前' },
          ],
        }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={status}
              onChange={setStatus}
              tabs={[
                ...queueStatuses.map((code) => ({ value: code, label: labelOf(code), count: counts[code] })),
                ...settledStatuses.map((code) => ({ value: code, label: labelOf(code) })),
                { value: 'ALL', label: '全部' },
              ]}
            />
            <WmsFilterSelect
              label="仓库"
              value={warehouse}
              onChange={setWarehouse}
              options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: `${w.warehouseCode} · ${w.name}` }))}
            />
            <WmsFilterSelect label="种类" value={kind} onChange={setKind} options={kindWords.map((word) => ({ value: word, label: labelOf(word) }))} />
          </div>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.arrivalId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [row.arrivalId])}
        pagination={pagination}
        emptyRowsNote="当前条件下没有匹配的待认领到货"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-unclaimed-arrivals',
          emptyTitle: status === 'UNCLAIMED' ? '没有待认领到货' : '还没有待认领记录',
          emptyDescription: '在「仓内作业台」登记无主到货，或预报取消/完结后再收货时，记录会出现在这里。',
          narrowed: settled.trim() !== '' || warehouse !== '' || kind !== '' || (status !== 'UNCLAIMED' && status !== 'ALL'),
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

/** 每行最右的那一格：这件到货此刻该做的一步。能就地做的是按钮，要去别处做的是链接，没有授予的只说在等什么。 */
function NextStepCell({
  row,
  rule,
  now,
  drawer,
}: {
  row: UnclaimedArrivalRecord;
  rule: DisposalRuleRecord | undefined;
  now: Date;
  drawer: ActionDrawerControl;
}) {
  const next = unclaimedNextStep(row, rule, now);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (next.kind === 'go') {
    return (
      <button
        type="button"
        className="text-[color:var(--ds-text-link-hover)] hover:underline"
        onClick={(event) => {
          event.stopPropagation();
          window.location.hash = `#/${next.moduleId}`;
        }}
      >
        {next.label}
      </button>
    );
  }
  const actionId = next.kind === 'claim' ? 'unclaimed-claim' : 'unclaimed-dispose';
  if (!drawer.has(actionId)) return <span className="text-idpxyz-textMuted">{next.kind === 'claim' ? '等认领' : '等处置'}</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={(event) => {
        // 行上的单击是预览：这一下只开表单，不顺带把行选进右栏。
        event.stopPropagation();
        drawer.open(actionId, next.kind === 'claim' ? claimPrefill(row) : { arrivalId: row.arrivalId });
      }}
    >
      {next.label}
    </Button>
  );
}
