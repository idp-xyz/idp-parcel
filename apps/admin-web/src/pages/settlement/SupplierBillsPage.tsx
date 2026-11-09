import { useEffect, useState } from 'react';
import { FilePlus2 } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { useRetryWhenChanged } from '../../query/answers';
import {
  WorkbenchPageTemplate,
  toggleWorkbenchSort,
  type ListCursorPaginationProps,
  type WorkbenchChip,
  type WorkbenchColumn,
  type WorkbenchSort,
} from '../../templates';
import { useTabViewState } from '../../templates/workspace-view-state';
import { labelFor } from '../../vocabulary/status';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState } from '../catalogue-view';
import { formatInstant } from '../moment';
import { hiddenDecisionHint, supplierBillReceptionDecision } from '../operator-grants';
import { listGroupLegalEntities } from '../party/api';
import { matchesSearch } from '../party/list-order';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword, type CatalogueCursor } from '../use-catalogue-cursor';
import { useMayMakeDecision } from '../use-operator-grants';
import { DocumentNumberCell } from '../visibility/DocumentNumberCell';
import { ExceptionDetailPanel, RowStatusPill } from '../visibility/ExceptionDetailPanel';
import { PageOnlyNote, PageOnlySearch } from '../visibility/page-only';
import {
  findSupplierBillReceptions,
  listAuditedPayables,
  listSettlementStatements,
  type ApiResult,
  type AuditedPayableRecord,
  type RegistryConditions,
  type SupplierBillReceptionRecord,
} from './api';
import {
  billListConditions,
  defaultBillSort,
  defaultPayableSort,
  pageEmptyNote,
  pageOnlyNote,
  pageStatusLabels,
  payableListConditions,
} from './list-conditions';
import { formatMoney } from './ops-ui';
import { namedRef, usePartyNames, type PartyNames } from './party-names';
import { SupplierBillPanel } from './SupplierBillPanel';
import { SupplierBillReceptionDialog } from './SupplierBillReceptionDialog';
import {
  billKey,
  claimOfBillKey,
  billLabel,
  billNumberOf,
  billProgress,
  billProgressOrder,
  matchSummary,
  operatorWords,
  payableLabel,
  payableNumberAbsenceNote,
  payableNumberName,
  payableNumberState,
  payableNumberStateOrder,
} from './supplier-bills';

// 供应商账单与应付（主责上下文 settlement-accounting「供应商主张与审核应付」，UC-SA-004；票 document-numbering/11）。两签：
// 供应商账单（GET /settlement-statements?registry=supplier-bill-reception，右栏读 GET /settlement-supplier-bill-lines）与审核应付
// （GET /settlement-audited-payables）。写入口是页头「登记供应商账单」与右栏逐行「审核这一行」「核实后再审」，各按自己那一种运营
// 决定授予收起（ADR-0315、ADR-0340、ADR-0357）。账单号是供应商的外部标识，取服务端账单号册上的、旧行从主张标识里取回
// （supplier-bills.ts 的 billNumberOf）；应付以应付单号为主列（ADR-0355 决定十一），应付标识进技术详情。
//
// 两本册已迁到 ADR-0144，各按游标翻页（工作台没有翻页槽，翻页条放工具条右端）。检索框下推成 `q`（条件在 list-conditions），
// 账单册覆盖主张、账单号、供应商编码、账期与公司主体编码，应付册覆盖应付标识、应付单号、主张、账单号、供应商编码与账期——
// 按应付单号在全册里找也走它。表头只给读口点名的可排维：账单的收到时间与账期、应付的审核时间。供应商与公司主体的名称不在读口
// 的列里，状态（账单走到哪一步、应付有没有单号）是页面从行上现算的词，读口不认：这两样留成只筛本页的小框与胶囊，胶囊不带计数，
// 旁边写明只筛本页；页头不再摆按状态的指标——一页里数出来的不是总数。
const info = moduleInfoById['supplier-bills'];

/**
 * 按了刷新（含审核、登记之后的重读）时，新答复回来之前接着摆上一次答到的那一页。useCatalogueCursor 重取时答复先回 null，工作台
 * 随之回加载态、收起右栏——账单右栏逐行审核的答复挂在右栏里，一收就丢。翻页与换条件照旧回加载态：手上那一页本来就不是要看的了。
 */
function useRefreshHold<Body>(cursor: CatalogueCursor<Body>): {
  answer: ApiResult<Body> | null;
  pagination: ListCursorPaginationProps | undefined;
  refreshing: boolean;
  refresh: () => void;
} {
  const [held, setHeld] = useState<Pick<CatalogueCursor<Body>, 'answer' | 'pagination'> | null>(null);
  useEffect(() => {
    if (cursor.answer !== null) setHeld(null);
  }, [cursor.answer]);
  const showHeld = cursor.answer === null && held !== null;
  return {
    answer: showHeld ? held.answer : cursor.answer,
    pagination: showHeld ? held.pagination : cursor.pagination,
    refreshing: cursor.answer === null,
    refresh: () => {
      if (cursor.answer?.kind === 'outcome') setHeld({ answer: cursor.answer, pagination: cursor.pagination });
      cursor.retry();
    },
  };
}

/** 只筛本页的状态胶囊：不带计数，再点一下回到全部。 */
function pageStatusChips(labels: readonly string[], active: string, setActive: (label: string) => void): WorkbenchChip[] {
  return [
    { id: 'ALL', label: '全部', active: active === 'ALL', onClick: () => setActive('ALL') },
    ...labels.map((label) => ({ id: label, label, active: active === label, onClick: () => setActive(active === label ? 'ALL' : label) })),
  ];
}

/** 公司主体编码 → 名称；读不到就显编码，不补假名。 */
function useLegalEntities(): { id: string; name: string }[] {
  const [entities, setEntities] = useState<{ id: string; name: string }[]>([]);
  useEffect(() => {
    let cancelled = false;
    void listGroupLegalEntities().then((answer) => {
      if (cancelled || answer.kind !== 'outcome') return;
      setEntities(
        answer.body.entities
          .filter((entity) => !entity.deactivatedAt)
          .map((entity) => ({ id: entity.legalEntityId, name: entity.partyNameKnown && entity.partyName ? entity.partyName : entity.legalEntityId })),
      );
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return entities;
}

// —— 供应商账单签 ——

const loadBills = (conditions: RegistryConditions<'supplier-bill-reception'>, after: string | null) =>
  listSettlementStatements(conditions, after);

function billColumns(names: PartyNames): WorkbenchColumn<SupplierBillReceptionRecord>[] {
  return [
    {
      key: 'bill',
      header: '账单号',
      render: (row) => (
        <span className="flex flex-col">
          <span className="truncate font-mono type-body leading-4 font-medium text-idpxyz-textBright" title={row.claim}>
            {billLabel(row)}
          </span>
          {row.claimVersion !== '1' ? <span className="type-caption text-idpxyz-breadcrumb">版本 {row.claimVersion}</span> : null}
        </span>
      ),
    },
    {
      key: 'supplier',
      header: '供应商',
      render: (row) => {
        const ref = namedRef(names.party(row.supplier), row.supplier);
        return (
          <span className="truncate text-idpxyz-text" title={row.supplier}>
            {ref.primary}
          </span>
        );
      },
    },
    { key: 'period', header: '账期', sortKey: 'period', render: (row) => <span className="font-mono text-idpxyz-breadcrumb">{row.period}</span> },
    {
      key: 'amount',
      header: '金额',
      right: true,
      render: (row) => <span className="font-mono tabular-nums text-idpxyz-textBright">{formatMoney(row.totalClaimedAmount, row.currency)}</span>,
    },
    { key: 'match', header: '对上情况', hideWhenDetailOpen: true, render: (row) => <span className="text-idpxyz-breadcrumb">{matchSummary(row)}</span> },
    { key: 'status', header: '状态', render: (row) => <RowStatusPill status={billProgress(row)} /> },
    {
      key: 'received',
      header: '收到时间',
      sortKey: 'received',
      hideWhenDetailOpen: true,
      render: (row) => (
        <time className="font-mono text-idpxyz-breadcrumb" dateTime={row.receivedAt}>
          {formatInstant(row.receivedAt)}
        </time>
      ),
    },
  ];
}

function SupplierBillsWorkbench({
  names,
  legalEntities,
  onAudited,
  onShowPayable,
}: {
  names: PartyNames;
  legalEntities: { id: string; name: string }[];
  onAudited: () => void;
  onShowPayable: (number: string) => void;
}) {
  const mayReceive = useMayMakeDecision(supplierBillReceptionDecision);
  const [search, setSearch] = useTabViewState('supplier-bills.bills.search', '');
  const [refine, setRefine] = useTabViewState('supplier-bills.bills.refine', '');
  const [status, setStatus] = useTabViewState<string>('supplier-bills.bills.status', 'ALL');
  const [sort, setSort] = useTabViewState<WorkbenchSort>('supplier-bills.bills.sort', defaultBillSort);
  const [selectedKey, setSelectedKey] = useTabViewState<string | null>('supplier-bills.bills.selected', null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const conditions = billListConditions(useSettledKeyword(search), sort);
  const { answer, pagination, refreshing, refresh: retry } = useRefreshHold(useCatalogueCursor(conditions, loadBills));

  const answered = answer?.kind === 'outcome';
  const rows = answer?.kind === 'outcome' ? answer.body.receptions : [];

  // 选中的账单不在这一页上（新登的那份收到时间早于页尾，或从别处点进来）：按主张在全册里单独读回它，右栏照样有东西可看。每次
  // 重读这一页也重读它，审过之后右栏的和跟着变。
  const [offPage, setOffPage] = useState<SupplierBillReceptionRecord | null>(null);
  const selectedOnPage = selectedKey !== null && rows.some((row) => billKey(row) === selectedKey);
  useEffect(() => {
    if (!answered || selectedKey === null || selectedOnPage) {
      setOffPage(null);
      return;
    }
    let cancelled = false;
    void findSupplierBillReceptions({ claim: claimOfBillKey(selectedKey) }).then((result) => {
      if (cancelled || result.kind !== 'outcome') return;
      setOffPage(result.body.receptions.find((row) => billKey(row) === selectedKey) ?? null);
    });
    return () => {
      cancelled = true;
    };
  }, [answered, answer, selectedKey, selectedOnPage]);
  const entityName = (id: string) => legalEntities.find((entity) => entity.id === id)?.name;
  const visible = rows.filter(
    (row) =>
      (status === 'ALL' || billProgress(row).label === status) &&
      matchesSearch(refine, [names.party(row.supplier), entityName(row.legalEntity)]),
  );
  const filteringOnPage = status !== 'ALL' || refine.trim() !== '';
  const labels = pageStatusLabels(rows, (row) => billProgress(row).label, billProgressOrder, status);

  return (
    <>
      <WorkbenchPageTemplate<SupplierBillReceptionRecord>
        title="供应商账单"
        viewStateKey="supplier-bills.bills"
        subtitle="供应商发来的账单与每一行对上了没有；对上的行审核通过才形成应付，对不上的要先核实。"
        onRefresh={retry}
        refreshing={refreshing}
        primaryAction={
          mayReceive ? (
            <Button size="sm" className={`${primaryActionClass} h-7 gap-1.5 type-body`} onClick={() => setDialogOpen(true)}>
              <FilePlus2 className="h-3.5 w-3.5" aria-hidden="true" />
              登记供应商账单
            </Button>
          ) : (
            <span className="type-body text-idpxyz-breadcrumb">{hiddenDecisionHint('登记供应商账单', labelFor(supplierBillReceptionDecision))}</span>
          )
        }
        search={{ value: search, onChange: setSearch, placeholder: '搜账单号、供应商编码、账期…' }}
        statusChips={answered ? pageStatusChips(labels, status, setStatus) : undefined}
        extraFilters={
          <>
            <PageOnlySearch value={refine} onChange={setRefine} subject="供应商名称、公司主体名称" />
            {answered ? <PageOnlyNote>{pageOnlyNote(rows.length, visible.length, filteringOnPage, '份')}</PageOnlyNote> : null}
          </>
        }
        toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
        columns={billColumns(names)}
        rows={visible}
        rowKey={billKey}
        selectedKey={selectedKey}
        onSelect={setSelectedKey}
        rowAttention={(row) => row.undecidedAmount !== '0' || row.openDisputeAmount !== '0'}
        sort={sort}
        onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
        noMatchText={pageEmptyNote('账单', rows.length)}
        onClearFilters={() => {
          setSearch('');
          setRefine('');
          setStatus('ALL');
        }}
        renderDetail={(key) => {
          const row =
            visible.find((candidate) => billKey(candidate) === key) ??
            rows.find((candidate) => billKey(candidate) === key) ??
            (offPage !== null && billKey(offPage) === key ? offPage : undefined);
          return row ? (
            <SupplierBillPanel
              row={row}
              names={names}
              legalEntityName={entityName(row.legalEntity)}
              onClose={() => setSelectedKey(null)}
              onAudited={() => {
                retry();
                onAudited();
              }}
              onShowPayable={onShowPayable}
            />
          ) : null;
        }}
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /settlement-statements?registry=supplier-bill-reception',
          emptyTitle: '还没有收到供应商账单',
          emptyDescription: mayReceive
            ? '供应商发来账单后，点页头「登记供应商账单」照来件录入；页面不会预置账单。'
            : '供应商账单录入之后在这里出现；页面不会预置账单。',
          narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
        })}
      />
      <SupplierBillReceptionDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        bills={rows}
        legalEntities={legalEntities}
        onReceived={(claim, version) => {
          retry();
          setStatus('ALL');
          setSelectedKey(billKey({ claim, claimVersion: version }));
        }}
      />
    </>
  );
}

// —— 审核应付签 ——

function payableColumns(names: PartyNames): WorkbenchColumn<AuditedPayableRecord>[] {
  return [
    {
      key: 'number',
      header: payableNumberName,
      render: (row) => <DocumentNumberCell documentNumber={row.documentNumber} internal={row.payable} absence={row.documentNumberAbsence} />,
    },
    {
      key: 'supplier',
      header: '供应商',
      render: (row) => (
        <span className="truncate text-idpxyz-text" title={row.supplier}>
          {namedRef(names.party(row.supplier), row.supplier).primary}
        </span>
      ),
    },
    {
      key: 'bill',
      header: '账单号 · 行',
      render: (row) => (
        <span className="truncate font-mono type-body leading-4 text-idpxyz-breadcrumb" title={row.claim}>
          {billNumberOf(row) ?? row.claim} · 第 {row.line} 行
        </span>
      ),
    },
    {
      key: 'amount',
      header: '金额',
      right: true,
      render: (row) => <span className="font-mono tabular-nums text-idpxyz-textBright">{formatMoney(row.amount, row.currency)}</span>,
    },
    { key: 'period', header: '账期', hideWhenDetailOpen: true, render: (row) => <span className="font-mono text-idpxyz-breadcrumb">{row.period}</span> },
    {
      key: 'audited',
      header: '审核时间',
      sortKey: 'audited',
      render: (row) => (
        <time className="font-mono text-idpxyz-breadcrumb" dateTime={row.auditedAt}>
          {formatInstant(row.auditedAt)}
        </time>
      ),
    },
  ];
}

function PayableDetail({
  row,
  names,
  legalEntityName,
  onClose,
}: {
  row: AuditedPayableRecord;
  names: PartyNames;
  legalEntityName: string | undefined;
  onClose: () => void;
}) {
  const note = payableNumberAbsenceNote(row);
  return (
    <ExceptionDetailPanel
      ariaLabel={`审核应付 ${payableLabel(row)}`}
      title={payableLabel(row)}
      status={payableNumberState(row)}
      subtitle={`审核于 ${formatInstant(row.auditedAt)}`}
      facts={[
        { label: '供应商', value: namedRef(names.party(row.supplier), row.supplier).primary },
        { label: '账单号', value: <span className="font-mono">{billNumberOf(row) ?? row.claim}</span> },
        { label: '账单上第几行', value: `第 ${row.line} 行` },
        { label: '金额', value: formatMoney(row.amount, row.currency) },
        ...(row.rangeFrom !== undefined && row.rangeTo !== undefined
          ? [{ label: '通过的那一段', value: `${row.resolvesDispute ? '争议再审通过 · ' : ''}区间 ${row.rangeFrom}–${row.rangeTo}` }]
          : []),
        { label: '账期', value: row.period },
        { label: '本公司主体', value: legalEntityName ?? row.legalEntity },
        { label: '应付归集账户', value: <span className="font-mono">{row.account}</span> },
        { label: '点审核的人', value: operatorWords(row.operationEvidence) },
        { label: '有权审核的一方', value: <span className="font-mono">{row.auditor}</span> },
      ]}
      technical={[
        { label: payableNumberName, value: row.documentNumber ?? note ?? '' },
        { label: '应付标识', value: row.payable },
        { label: '主张标识', value: row.claim },
        { label: '预期成本版本', value: row.expectedVersion },
        { label: '审核授权方', value: row.auditor },
        { label: '审核操作证据', value: row.operationEvidence ?? '' },
        { label: '应付归集账户', value: row.account },
        { label: '公司主体编码', value: row.legalEntity },
        { label: '登记时间', value: formatInstant(row.recordedAt) },
      ].filter((item) => item.value !== '')}
      onClose={onClose}
    >
      {note ? <p>这笔应付没有{payableNumberName}：{note}。</p> : null}
      <p>审核应付说的是这笔钱审过了、该付；不表示已经付给供应商。付款与核销在「收付款核销」。</p>
    </ExceptionDetailPanel>
  );
}

function AuditedPayablesWorkbench({
  names,
  legalEntities,
  search,
  setSearch,
  reloadKey,
}: {
  names: PartyNames;
  legalEntities: { id: string; name: string }[];
  search: string;
  setSearch: (value: string) => void;
  reloadKey: number;
}) {
  const [refine, setRefine] = useTabViewState('supplier-bills.payables.refine', '');
  const [status, setStatus] = useTabViewState<string>('supplier-bills.payables.status', 'ALL');
  const [sort, setSort] = useTabViewState<WorkbenchSort>('supplier-bills.payables.sort', defaultPayableSort);
  const [selectedKey, setSelectedKey] = useTabViewState<string | null>('supplier-bills.payables.selected', null);
  const conditions = payableListConditions(useSettledKeyword(search), sort);
  const { answer, pagination, refreshing, refresh: retry } = useRefreshHold(useCatalogueCursor(conditions, listAuditedPayables));
  // 账单签审过一行，应付册就多了一笔，跟着重读。
  useRetryWhenChanged(reloadKey, retry);

  const answered = answer?.kind === 'outcome';
  const rows = answer?.kind === 'outcome' ? answer.body.payables : [];
  const entityName = (id: string) => legalEntities.find((entity) => entity.id === id)?.name;
  const visible = rows.filter(
    (row) => (status === 'ALL' || payableNumberState(row).label === status) && matchesSearch(refine, [names.party(row.supplier)]),
  );
  const filteringOnPage = status !== 'ALL' || refine.trim() !== '';
  const labels = pageStatusLabels(rows, (row) => payableNumberState(row).label, payableNumberStateOrder, status);

  return (
    <WorkbenchPageTemplate<AuditedPayableRecord>
      title="审核应付"
      viewStateKey="supplier-bills.payables"
      subtitle={`供应商账单审核通过的金额，按${payableNumberName}列；审过了该付，不表示已经付款。`}
      onRefresh={retry}
      refreshing={refreshing}
      search={{ value: search, onChange: setSearch, placeholder: `搜${payableNumberName}、账单号、供应商编码、账期…` }}
      statusChips={answered ? pageStatusChips(labels, status, setStatus) : undefined}
      extraFilters={
        <>
          <PageOnlySearch value={refine} onChange={setRefine} subject="供应商名称" />
          {answered ? <PageOnlyNote>{pageOnlyNote(rows.length, visible.length, filteringOnPage, '笔')}</PageOnlyNote> : null}
        </>
      }
      toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
      columns={payableColumns(names)}
      rows={visible}
      rowKey={(row) => row.payable}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      sort={sort}
      onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
      noMatchText={pageEmptyNote('应付', rows.length)}
      onClearFilters={() => {
        setSearch('');
        setRefine('');
        setStatus('ALL');
      }}
      renderDetail={(key) => {
        const row = visible.find((candidate) => candidate.payable === key);
        return row ? <PayableDetail row={row} names={names} legalEntityName={entityName(row.legalEntity)} onClose={() => setSelectedKey(null)} /> : null;
      }}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /settlement-audited-payables',
        emptyTitle: '还没有审核应付',
        emptyDescription: '在「供应商账单」签里点开一张账单，对上的行审核通过后形成应付，在这里出现。',
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
      })}
    />
  );
}

type SupplierBillsTab = 'bills' | 'payables';

/** 供应商账单与应付：两签各接一本册；账单签右栏审核形成的应付，在应付签里按应付单号可查。 */
export function SupplierBillsPage() {
  const names = usePartyNames();
  const legalEntities = useLegalEntities();
  const [tab, setTab] = useTabViewState<SupplierBillsTab>('supplier-bills.tab', 'bills');
  const [payableSearch, setPayableSearch] = useTabViewState('supplier-bills.payables.search', '');
  const [payablesReload, setPayablesReload] = useState(0);

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={(value) => setTab(value === 'payables' ? 'payables' : 'bills')} className="flex flex-1 flex-col gap-0 overflow-hidden">
        <TabsList className="shrink-0 px-4">
          <TabsTrigger value="bills">供应商账单</TabsTrigger>
          <TabsTrigger value="payables">审核应付</TabsTrigger>
        </TabsList>
        <TabsContent value="bills" className="flex flex-1 flex-col overflow-hidden data-[state=inactive]:hidden">
          <SupplierBillsWorkbench
            names={names}
            legalEntities={legalEntities}
            onAudited={() => setPayablesReload((value) => value + 1)}
            onShowPayable={(number) => {
              setPayableSearch(number);
              setTab('payables');
            }}
          />
        </TabsContent>
        <TabsContent value="payables" className="flex flex-1 flex-col overflow-hidden data-[state=inactive]:hidden">
          <AuditedPayablesWorkbench
            names={names}
            legalEntities={legalEntities}
            search={payableSearch}
            setSearch={setPayableSearch}
            reloadKey={payablesReload}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
