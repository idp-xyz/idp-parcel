import { useEffect, useState } from 'react';
import { FilePlus2 } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { feedbackFor } from '../../components/action-feedback';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import { useTabViewState } from '../../templates/workspace-view-state';
import { labelFor } from '../../vocabulary/status';
import { catalogueViewState } from '../catalogue-view';
import { formatInstant } from '../moment';
import { hiddenDecisionHint, supplierBillReceptionDecision } from '../operator-grants';
import { listGroupLegalEntities } from '../party/api';
import { matchesSearch } from '../party/list-order';
import { useMayMakeDecision } from '../use-operator-grants';
import { DocumentNumberCell } from '../visibility/DocumentNumberCell';
import { ExceptionDetailPanel, RowStatusPill } from '../visibility/ExceptionDetailPanel';
import { countByStatus, sortRowsBy, type SortAccessor } from '../visibility/exception-list';
import { statusFilterParts } from '../visibility/status-filter';
import {
  listAuditedPayables,
  listSettlementStatements,
  type ApiResult,
  type AuditedPayableListResponseBody,
  type AuditedPayableRecord,
  type SupplierBillReceptionListResponseBody,
  type SupplierBillReceptionRecord,
} from './api';
import { formatMoney } from './ops-ui';
import { namedRef, usePartyNames, type PartyNames } from './party-names';
import { SupplierBillPanel } from './SupplierBillPanel';
import { SupplierBillReceptionDialog } from './SupplierBillReceptionDialog';
import {
  billKey,
  billLabel,
  billNumberOf,
  billProgress,
  matchSummary,
  operatorWords,
  pageCountSummary,
  payableLabel,
  payableNumberAbsenceNote,
  payableNumberName,
  payableNumberState,
  withExactHits,
} from './supplier-bills';

// 供应商账单与应付（主责上下文 settlement-accounting「供应商主张与审核应付」，UC-SA-004；票 document-numbering/11）。两签：
// 供应商账单（GET /settlement-statements?registry=supplier-bill-reception，右栏读 GET /settlement-supplier-bill-lines）与审核应付
// （GET /settlement-audited-payables）。写入口是页头「登记供应商账单」与右栏逐行「审核通过」，各按自己那一种运营决定授予收起
// （ADR-0315、ADR-0340）。账单号是供应商的外部标识，从主张标识里取回（supplier-bills.ts 的 billNumberOf）；应付以应付单号为主列
// （ADR-0355 决定十一），应付标识进技术详情。
const info = moduleInfoById['supplier-bills'];

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

const billSort: Record<string, SortAccessor<SupplierBillReceptionRecord>> = {
  bill: { value: billLabel },
  period: { value: (row) => row.period },
  received: { value: (row) => row.receivedAt, instant: true },
};

function billColumns(names: PartyNames): WorkbenchColumn<SupplierBillReceptionRecord>[] {
  return [
    {
      key: 'bill',
      header: '账单号',
      sortKey: 'bill',
      render: (row) => (
        <span className="flex flex-col">
          <span className="truncate font-mono text-xs font-medium text-idpxyz-textBright" title={row.claim}>
            {billLabel(row)}
          </span>
          {row.claimVersion !== '1' ? <span className="text-[11px] text-idpxyz-breadcrumb">版本 {row.claimVersion}</span> : null}
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
  const [status, setStatus] = useTabViewState<string>('supplier-bills.bills.status', 'ALL');
  const [sort, setSort] = useTabViewState<WorkbenchSort>('supplier-bills.bills.sort', { key: 'received', dir: -1 });
  const [selectedKey, setSelectedKey] = useTabViewState<string | null>('supplier-bills.bills.selected', null);
  const [answer, setAnswer] = useState<ApiResult<SupplierBillReceptionListResponseBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setAnswer((previous) => (previous?.kind === 'outcome' ? previous : null));
    void listSettlementStatements('supplier-bill-reception').then((result) => {
      if (cancelled) return;
      setAnswer(result);
      setRefreshing(false);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  const retry = () => {
    setRefreshing(true);
    setReloadToken((token) => token + 1);
  };
  const answered = answer?.kind === 'outcome';
  const rows = answer?.kind === 'outcome' ? answer.body.receptions : [];
  const truncated = answer?.kind === 'outcome' && answer.body.truncated;
  const entityName = (id: string) => legalEntities.find((entity) => entity.id === id)?.name;
  const counts = countByStatus(rows, billProgress);
  const visible = sortRowsBy(
    rows.filter(
      (row) =>
        (status === 'ALL' || billProgress(row).label === status) &&
        matchesSearch(search, [billLabel(row), row.claim, row.supplier, names.party(row.supplier), row.period, row.legalEntity, entityName(row.legalEntity)]),
    ),
    sort,
    billSort,
  );
  const filter = answered ? statusFilterParts(counts, rows.length, status, setStatus) : undefined;

  return (
    <>
      <WorkbenchPageTemplate<SupplierBillReceptionRecord>
        title="供应商账单"
        viewStateKey="supplier-bills.bills"
        subtitle="供应商发来的账单与每一行对上了没有；对上的行审核通过才形成应付，对不上的要先核实。"
        kpis={filter?.kpis}
        onRefresh={retry}
        refreshing={refreshing}
        primaryAction={
          mayReceive ? (
            <Button size="sm" className={`${primaryActionClass} h-7 gap-1.5 text-[12px]`} onClick={() => setDialogOpen(true)}>
              <FilePlus2 className="h-3.5 w-3.5" aria-hidden="true" />
              登记供应商账单
            </Button>
          ) : (
            <span className="text-[12px] text-idpxyz-breadcrumb">{hiddenDecisionHint('登记供应商账单', labelFor(supplierBillReceptionDecision))}</span>
          )
        }
        search={{ value: search, onChange: setSearch, placeholder: '搜账单号、供应商、账期…' }}
        statusChips={filter?.chips}
        toolbarRight={answered ? pageCountSummary(rows.length, visible.length, truncated, '份', '收到') : undefined}
        columns={billColumns(names)}
        rows={visible}
        rowKey={billKey}
        selectedKey={selectedKey}
        onSelect={setSelectedKey}
        rowAttention={(row) => row.matchedWithoutPayableCount > 0}
        sort={sort}
        onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
        noMatchText="当前筛选条件下没有匹配的账单"
        onClearFilters={() => {
          setSearch('');
          setStatus('ALL');
        }}
        renderDetail={(key) => {
          const row = visible.find((candidate) => billKey(candidate) === key);
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

const payableSort: Record<string, SortAccessor<AuditedPayableRecord>> = {
  number: { value: (row) => row.documentNumber ?? row.payable },
  audited: { value: (row) => row.auditedAt, instant: true },
};

function payableColumns(names: PartyNames): WorkbenchColumn<AuditedPayableRecord>[] {
  return [
    {
      key: 'number',
      header: payableNumberName,
      sortKey: 'number',
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
        <span className="truncate font-mono text-xs text-idpxyz-breadcrumb" title={row.claim}>
          {billNumberOf(row.claim, row.supplier) ?? row.claim} · 第 {row.line} 行
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
        { label: '账单号', value: <span className="font-mono">{billNumberOf(row.claim, row.supplier) ?? row.claim}</span> },
        { label: '账单上第几行', value: `第 ${row.line} 行` },
        { label: '金额', value: formatMoney(row.amount, row.currency) },
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
  const [status, setStatus] = useTabViewState<string>('supplier-bills.payables.status', 'ALL');
  const [sort, setSort] = useTabViewState<WorkbenchSort>('supplier-bills.payables.sort', { key: 'audited', dir: -1 });
  const [selectedKey, setSelectedKey] = useTabViewState<string | null>('supplier-bills.payables.selected', null);
  const [answer, setAnswer] = useState<ApiResult<AuditedPayableListResponseBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  // 检索词同时当应付单号去服务端全册精确检索：页只取最近的那些，页外的应付只有这一问找得到（ADR-0355 决定十一）。
  const term = search.trim();
  const [exact, setExact] = useState<{ term: string; result: ApiResult<AuditedPayableListResponseBody> } | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer((previous) => (previous?.kind === 'outcome' ? previous : null));
    void listAuditedPayables().then((result) => {
      if (cancelled) return;
      setAnswer(result);
      setRefreshing(false);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken, reloadKey]);

  useEffect(() => {
    if (term === '') {
      setExact(null);
      return;
    }
    let cancelled = false;
    const timer = window.setTimeout(() => {
      void listAuditedPayables({ documentNumber: term }).then((result) => {
        if (!cancelled) setExact({ term, result });
      });
    }, 250);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [term, reloadToken, reloadKey]);

  const retry = () => {
    setRefreshing(true);
    setReloadToken((token) => token + 1);
  };
  const answered = answer?.kind === 'outcome';
  const rows = answer?.kind === 'outcome' ? answer.body.payables : [];
  const truncated = answer?.kind === 'outcome' && answer.body.truncated;
  const exactNow = exact?.term === term ? exact.result : null;
  const pool = withExactHits(rows, exactNow?.kind === 'outcome' ? exactNow.body.payables : []);
  const entityName = (id: string) => legalEntities.find((entity) => entity.id === id)?.name;
  const counts = countByStatus(rows, payableNumberState);
  const visible = sortRowsBy(
    pool.filter(
      (row) =>
        (status === 'ALL' || payableNumberState(row).label === status) &&
        matchesSearch(search, [
          row.documentNumber,
          row.payable,
          row.claim,
          billNumberOf(row.claim, row.supplier),
          row.supplier,
          names.party(row.supplier),
          row.period,
        ]),
    ),
    sort,
    payableSort,
  );
  const filter = answered ? statusFilterParts(counts, rows.length, status, setStatus) : undefined;

  return (
    <WorkbenchPageTemplate<AuditedPayableRecord>
      title="审核应付"
      viewStateKey="supplier-bills.payables"
      subtitle={`供应商账单审核通过的金额，按${payableNumberName}列；审过了该付，不表示已经付款。`}
      kpis={filter?.kpis}
      onRefresh={retry}
      refreshing={refreshing}
      search={{ value: search, onChange: setSearch, placeholder: `搜${payableNumberName}、账单号、供应商…` }}
      statusChips={filter?.chips}
      toolbarRight={
        answered
          ? `${pageCountSummary(rows.length, visible.length, truncated, '笔', '审核')}${
              exactNow && exactNow.kind !== 'outcome'
                ? `；按${payableNumberName}在全册里检索没有答出（${feedbackFor(exactNow, '检索').text}），只在这一页里找`
                : truncated
                  ? `；按完整的${payableNumberName}搜，页外的也找得到`
                  : ''
            }`
          : undefined
      }
      columns={payableColumns(names)}
      rows={visible}
      rowKey={(row) => row.payable}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      sort={sort}
      onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
      noMatchText="当前筛选条件下没有匹配的应付"
      onClearFilters={() => {
        setSearch('');
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
