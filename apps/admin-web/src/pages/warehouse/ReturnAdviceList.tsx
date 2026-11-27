import { useMemo, type ReactNode } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import type { ReturnPreAdviceRecord, ReturnReceiptRecord } from './api';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { returnAdviceKind, returnsModule as moduleId } from './return-address';
import {
  adviceInFilter,
  adviceRows,
  countByStatus,
  isReturnOrder,
  returnAdviceNextStep,
  returnAdviceNumber,
  returnAdviceWaitingSince,
  sortAdviceRows,
  type AdviceRow,
  type ReturnAdviceStatus,
} from './return-flow';
import { ReturnAdviceDocument } from './ReturnAdviceDocument';
import type { QueueFilters } from './use-queue-filters';
import type { ReturnsOverviewState, ReturnsPanel } from './use-returns-overview';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const info = moduleInfoById[moduleId];
const receivedKey = 'receivedAt';
const allQueues = 'ALL';

type Row = AdviceRow<ReturnPreAdviceRecord, ReturnReceiptRecord>;

// 签的先后照一张预报往下走的先后。预报自己没有状态：状态是拿到仓记录对出来的，概览整份都在手里，所以每个签都如实写数，
// 包括已处置——不像按状态问读口的列表，这里数一个签不多花一次请求。
const statusCodes: readonly ReturnAdviceStatus[] = ['AWAITING_ARRIVAL', 'ARRIVED', 'DISPOSED'];

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/**
 * 退货预报：货主或运营事先登记的退回明细。点一行在右边看这张预报，预报号仍进整页。概览读口不分页、不接筛选维，
 * 状态、数量、货主与仓的筛选都在已取回的整份概览上做，所以没有翻页条。
 */
export function ReturnAdviceList({ overview, filters }: { overview: ReturnsOverviewState; filters: QueueFilters }) {
  const drawer = useActionDrawer(receivingActionSpecs);
  const { answer, reload, reloading } = overview;
  const [openKey, setOpenKey] = useTabViewState<string | null>('warehouse-returns.advice.open', null);
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const all = useMemo(() => adviceRows(body?.preAdvices ?? [], body?.receipts ?? []), [body]);
  const loaded = body !== null;

  // 数量是当前检索词、货主与仓之下各状态各有几张；点哪一队只换看的那一队，不改数量。
  const inScope = all.filter((row) => adviceInFilter(row.advice, filters));
  const counts = countByStatus(inScope, (row) => row.status);
  const shown = filters.status === allQueues ? inScope : inScope.filter((row) => row.status === filters.status);
  // 选了待到仓这一队，按受理先后排，等得最久的在最上面；看别的，最新的在上面。人点过表头就照他的。
  const defaultOrder = filters.status === 'AWAITING_ARRIVAL' ? 'oldest' : 'newest';
  const order = isReturnOrder(filters.sort) ? filters.sort : defaultOrder;
  const rows = sortAdviceRows(shown, order);
  const sort: WorkbenchSort = { key: receivedKey, dir: order === 'oldest' ? 1 : -1 };
  const now = new Date();
  const ownerNames = useMemo(
    () => new Map(drawer.lookups.owners.map((account) => [account.accountId, account.customerPartyName ?? ''])),
    [drawer.lookups.owners],
  );
  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((warehouse) => [warehouse.warehouseCode, warehouse.name ?? ''])),
    [drawer.lookups.warehouses],
  );
  const named = (names: ReadonlyMap<string, string>, code: string) => {
    const name = nameOf(names, code);
    return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
  };

  // 开了详情栏之后列表变窄，次要的列收起：预报号、状态、已等与下一步一直在。
  const columns: WorkbenchColumn<Row>[] = [
    {
      key: 'id',
      header: '预报号',
      width: fitColumnWidth,
      render: (row) => (
        <WmsTwoLine
          primary={<DocumentLink moduleId={moduleId} parts={[returnAdviceKind, returnAdviceNumber(row.advice)]}>{returnAdviceNumber(row.advice)}</DocumentLink>}
          secondary={shortInstant(row.advice.receivedAt, now)}
          title={`受理于 ${formatInstant(row.advice.receivedAt)}`}
        />
      ),
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} /> },
    { key: 'owner', header: '货主', width: fitColumnWidth, minListWidth: 900, render: (row) => named(ownerNames, row.advice.ownerAccount) },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 1060, render: (row) => named(warehouseNames, row.advice.warehouseCode) },
    {
      key: 'track',
      header: '退件单号',
      minListWidth: 760,
      render: (row) => <WmsTwoLine fill primary={<span className="font-mono">{row.advice.trackingNumber}</span>} title={row.advice.trackingNumber} />,
    },
    {
      key: 'lines',
      header: nowrap('预计行'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1100,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{row.advice.lines?.length ?? 0}</span>,
    },
    { key: 'origin', header: '来源', width: fitColumnWidth, minListWidth: 1160, render: (row) => nowrap(labelOf(row.advice.origin)) },
    {
      key: 'waiting',
      header: nowrap('已等'),
      right: true,
      width: fitColumnWidth,
      sortKey: receivedKey,
      render: (row) => {
        const since = returnAdviceWaitingSince(row.status, row.advice);
        return <span className="whitespace-nowrap tabular-nums">{elapsedLabel(since, now) || '—'}</span>;
      },
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <NextStepCell status={row.status} /> },
  ];

  const queueStats: WmsQueueStat[] = statusCodes.map((code) => ({
    id: code,
    label: labelOf(code),
    count: loaded ? (counts[code] ?? 0) : null,
    active: filters.status === code,
    onClick: () => filters.setStatus(filters.status === code ? allQueues : code),
  }));
  const narrowed = filters.keyword.trim() !== '' || filters.owner !== '' || filters.warehouse !== '' || filters.status !== allQueues || filters.sort !== null;
  const clearFilters = () => {
    filters.setKeyword('');
    filters.setOwner('');
    filters.setWarehouse('');
    filters.setStatus(allQueues);
    filters.setSort(null);
  };
  // 概览是整册：整册都没有预报才是空态。筛没了时行是空的、整册不是，模板改说「当前筛选下没有」。
  const viewState: TemplateViewState = catalogueViewState(answer, all.length, reload, {
    module: info,
    endpoint: 'GET /warehouse-fulfillment-returns-overview',
    emptyTitle: '还没有退货预报',
    emptyDescription: '点右上角「代录退货预报」登记，或等货主经客户接口提交。退件到仓后，状态会跟着变。',
  });
  const panel: ReturnsPanel = { onClose: () => setOpenKey(null), onChanged: reload, overview: answer, refreshing: reloading };

  return (
    <>
      <WorkbenchPageTemplate<Row>
        title="退货预报"
        viewStateKey="warehouse-returns-advices"
        subtitle="点一行在右边看这张预报、做它的下一步"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={reload}
        refreshing={reloading}
        primaryAction={
          drawer.has('return-pre-advice') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('return-pre-advice')}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              代录退货预报
            </Button>
          ) : undefined
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜预报号、退件单号、货主或仓' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={filters.status === allQueues ? '' : filters.status}
              onChange={(next) => filters.setStatus(next === '' ? allQueues : next)}
              options={statusCodes.map((code) => ({ value: code, label: labelOf(code) }))}
            />
            <WmsFilterSelect
              label="货主"
              value={filters.owner}
              onChange={filters.setOwner}
              options={drawer.lookups.owners.map((account) => ({
                value: account.accountId,
                label: account.customerPartyName ? `${account.customerPartyName} · ${account.accountId}` : account.accountId,
              }))}
            />
            <WmsFilterSelect
              label="仓库"
              value={filters.warehouse}
              onChange={filters.setWarehouse}
              options={drawer.lookups.warehouses.map((warehouse) => ({
                value: warehouse.warehouseCode,
                label: warehouse.name ? `${warehouse.name} · ${warehouse.warehouseCode}` : warehouse.warehouseCode,
              }))}
            />
            {narrowed ? (
              <button
                type="button"
                className="whitespace-nowrap type-body text-idpxyz-textMuted hover:text-idpxyz-textBright focus-visible:underline focus-visible:outline-none"
                onClick={clearFilters}
              >
                清除
              </button>
            ) : null}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.advice.adviceId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => {
          const next = toggleWorkbenchSort(sort, key);
          filters.setSort(next.dir === 1 ? 'oldest' : 'newest');
        }}
        noMatchText="当前筛选下没有退货预报"
        onClearFilters={clearFilters}
        renderDetail={(key) => {
          const row = all.find((item) => item.advice.adviceId === key);
          return <ReturnAdviceDocument adviceKey={row ? returnAdviceNumber(row.advice) : key} panel={panel} />;
        }}
        viewState={viewState}
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
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        // 行上的单击是开详情：这一下只去作业台，不顺带把详情栏开合一次。
        event.stopPropagation();
        window.location.hash = `#/${next.moduleId}`;
      }}
    >
      {next.label}
    </Button>
  );
}
