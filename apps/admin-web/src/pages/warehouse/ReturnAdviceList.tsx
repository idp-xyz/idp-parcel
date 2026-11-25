import { useMemo } from 'react';
import { Plus, RefreshCw } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import type { ReturnPreAdviceRecord, ReturnReceiptRecord } from './api';
import { DocumentLink } from './document-page';
import { returnAdviceKind, returnsModule as moduleId } from './return-address';
import {
  adviceInFilter,
  adviceRegistrant,
  adviceRows,
  countByStatus,
  isReturnOrder,
  returnAdviceNextStep,
  returnAdviceNumber,
  returnAdviceWaitingSince,
  sortAdviceRows,
  type AdviceRow,
  type ReturnAdviceStatus,
  type ReturnOrder,
} from './return-flow';
import { openDocument } from './use-document';
import type { QueueFilters } from './use-queue-filters';
import type { ReturnsOverviewState } from './use-returns-overview';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

type Row = AdviceRow<ReturnPreAdviceRecord, ReturnReceiptRecord>;

// 签的先后照一张预报往下走的先后。预报自己没有状态：状态是拿到仓记录对出来的，概览整份都在手里，所以每个签都如实写数，
// 包括已处置与「全部」——不像按状态问读口的列表，这里数一个签不多花一次请求。
const statusCodes: readonly ReturnAdviceStatus[] = ['AWAITING_ARRIVAL', 'ARRIVED', 'DISPOSED'];

const goModule = (target: string) => {
  window.location.hash = `#/${target}`;
};

/**
 * 退货预报：货主或运营事先登记的退回明细。点预报号打开退货预报。概览读口不分页、不接筛选维，状态、数量、货主与仓的筛选
 * 都在已取回的整份概览上做；要看别处刚发生的变动（比如作业台刚收的退件），点刷新重读。
 */
export function ReturnAdviceList({ overview, filters }: { overview: ReturnsOverviewState; filters: QueueFilters }) {
  const drawer = useActionDrawer(receivingActionSpecs);
  const { answer, reload } = overview;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const all = useMemo(() => adviceRows(body?.preAdvices ?? [], body?.receipts ?? []), [body]);
  const loaded = body !== null;

  // 数量是当前检索词、货主与仓之下各状态各有几张；点哪个签只换看的那一队，不改数量。
  const inScope = all.filter((row) => adviceInFilter(row.advice, filters));
  const counts = countByStatus(inScope, (row) => row.status);
  const shown = filters.status === 'ALL' ? inScope : inScope.filter((row) => row.status === filters.status);
  // 选了待到仓这一队，按受理先后排，等得最久的在最上面；看别的，最新的在上面。人改过排序就照他的。
  const defaultOrder: ReturnOrder = filters.status === 'AWAITING_ARRIVAL' ? 'oldest' : 'newest';
  const order = isReturnOrder(filters.sort) ? filters.sort : defaultOrder;
  const rows = sortAdviceRows(shown, order);
  const now = new Date();

  const columns: ListColumn<Row>[] = [
    {
      id: 'id',
      header: '预报号',
      className: 'whitespace-nowrap',
      render: (row) => <DocumentLink moduleId={moduleId} parts={[returnAdviceKind, returnAdviceNumber(row.advice)]} />,
    },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
    { id: 'track', header: '退件单号', className: mono, render: (row) => row.advice.trackingNumber },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.advice.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.advice.warehouseCode },
    { id: 'lines', header: '预计行', align: 'right', className: mono, render: (row) => String(row.advice.lines?.length ?? 0) },
    { id: 'origin', header: '来源', className: 'whitespace-nowrap', render: (row) => labelOf(row.advice.origin) },
    { id: 'waiting', header: '已等', align: 'right', className: 'whitespace-nowrap', render: (row) => elapsedLabel(returnAdviceWaitingSince(row.status, row.advice), now) || '—' },
    { id: 'next', header: '下一步', className: 'whitespace-nowrap', render: (row) => <NextStepCell status={row.status} /> },
  ];

  const inspectorOf = (row: Row): InspectorContent => {
    const { advice } = row;
    const number = returnAdviceNumber(advice);
    const next = returnAdviceNextStep(row.status);
    const actions: InspectorAction[] = [{ label: '打开退货预报', onRun: () => openDocument(moduleId, [returnAdviceKind, number]) }];
    if (next !== null) actions.push({ label: next.label, onRun: () => goModule(next.moduleId) });
    return {
      title: number,
      subtitle: advice.trackingNumber,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主 / 仓', value: `${advice.ownerAccount} · ${advice.warehouseCode}`, mono: true },
            { label: '预计行数', value: String(advice.lines?.length ?? 0), mono: true },
            { label: '到仓记录', value: row.receipts.length > 0 ? `${row.receipts.length} 笔` : '还没有', mono: true },
            { label: '来源', value: labelOf(advice.origin) },
            { label: '依据', value: advice.basis || '—', mono: true },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: labelOf(row.status) }] },
        { kind: 'actions', actions },
        { kind: 'audit', fields: [{ label: '登记人 / 受理时刻', value: `${adviceRegistrant(advice)} · ${formatInstant(advice.receivedAt)}`, mono: true }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<Row>
        moduleId={moduleId}
        title="退货预报"
        description="货主或运营事先登记的退回明细，预计商品不入库存账。数量是当前筛选下各状态有几张：没有对上到仓记录的是待到仓，到仓的货都记了去向的是已处置。点预报号看每一行到了没有。"
        headerActions={
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" onClick={reload}>
              <RefreshCw className="mr-1 h-3.5 w-3.5" />
              刷新
            </Button>
            {drawer.has('return-pre-advice') ? (
              <Button type="button" className={primaryActionClass} onClick={() => drawer.open('return-pre-advice')}>
                <Plus className="mr-1 h-3.5 w-3.5" />
                代录退货预报
              </Button>
            ) : null}
          </div>
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜预报号、退件单号、货主或仓' }}
        sort={{
          value: order,
          onChange: filters.setSort,
          options: [
            { value: 'newest', label: '最新受理的在前' },
            { value: 'oldest', label: '等得最久的在前' },
          ],
        }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={filters.status}
              onChange={filters.setStatus}
              tabs={[
                ...statusCodes.map((code) => ({ value: code, label: labelOf(code), count: loaded ? (counts[code] ?? 0) : undefined })),
                { value: 'ALL', label: '全部', count: loaded ? inScope.length : undefined },
              ]}
            />
            <WmsFilterSelect
              label="货主"
              value={filters.owner}
              onChange={filters.setOwner}
              options={drawer.lookups.owners.map((account) => ({
                value: account.accountId,
                label: account.customerPartyName ? `${account.accountId} · ${account.customerPartyName}` : account.accountId,
              }))}
            />
            <WmsFilterSelect
              label="仓库"
              value={filters.warehouse}
              onChange={filters.setWarehouse}
              options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: `${w.warehouseCode} · ${w.name}` }))}
            />
          </div>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.advice.adviceId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [returnAdviceKind, returnAdviceNumber(row.advice)])}
        emptyRowsNote="当前条件下没有退货预报"
        // 概览是整册：整册都没有预报才是空态，筛没了说的是「这组条件下没有」，由 emptyRowsNote 说。
        viewState={catalogueViewState(answer, all.length, reload, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-returns-overview',
          emptyTitle: '还没有退货预报',
          emptyDescription: '点右上角「代录退货预报」登记，或等货主经客户接口提交。退件到仓后，状态会跟着变。',
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={reload} />
    </>
  );
}

/** 每行最右的那一格：这张预报此刻该做的一步。收退件在作业台做，这里只指路；到了仓的预报，下一步在它的到仓记录上。 */
function NextStepCell({ status }: { status: ReturnAdviceStatus }) {
  const next = returnAdviceNextStep(status);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  return (
    <button
      type="button"
      className="text-[color:var(--ds-text-link-hover)] hover:underline"
      onClick={(event) => {
        // 行上的单击是预览：这一下只去作业台，不顺带把行选进右栏。
        event.stopPropagation();
        goModule(next.moduleId);
      }}
    >
      {next.label}
    </button>
  );
}
