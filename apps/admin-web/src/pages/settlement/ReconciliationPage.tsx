import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { MultiRegistrationPanel } from '../../components/registration';
import { statementDecisionTargets } from './decisions';
import { problemNote as decisionProblemNote } from './presentation';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import type { ApiResult } from '../catalogue-api';
import { catalogueConditionsNarrow, type CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { matchesSearch } from '../party/list-order';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import { PageOnlySearch } from '../visibility/page-only';
import {
  listDisputedCustomerStatements,
  listSettlementCharges,
  listSettlementStatements,
  registryConditions,
  type CustomerStatementListResponseBody,
  type DisputedCustomerStatementListResponseBody,
  type CustomerStatementRecord,
} from './api';
import { isStatementRegistryConditions, pageEmptyNote, statementListConditions, statementSummary } from './list-conditions';
import { TwoLine, formatMoney, technicalRows } from './ops-ui';
import { accountCounterparties } from './ops-refs';
import { namedRef, usePartyNames, type PartyNames } from './party-names';
import { disputedOnlyOf, reconciliationHash } from './reconciliation-address';
import { StatementDisputePanel } from './StatementDisputePanel';
import { disputeLabel, disputeNumberName, openDisputesOf } from './dispute-forms';

// 对账单页（主责上下文 settlement-accounting「结算账户、对账与争议」）。客户对账单接 GET /settlement-statements?registry=customer-statement；
// 「只看有未了结异议的」换接 GET /settlement-disputed-statements。供应商账单接收册只在「供应商账单与应付」页（SupplierBillsPage）列：
// 两处列同一本册，就会各说一套状态与指标。
//
// 两个读口都已迁到 ADR-0144，按游标翻页（模板的翻页条）；勾不勾「只看待对账」是两问，切换即回第一页。检索框下推成 `q`，读口覆盖
// 对账单号、结算账户、账期与单上的争议号；「不看已作废的单」下推成 voided 一维，两个读口都认。客户名称要经费用册与参与方册两跳转写，
// 不在读口的列里——按它找留一个只筛本页的小框，字面写明。条数只数这一页，字面写「本页」；争议张数是读口答的整个队列。
const info = moduleInfoById['reconciliation'];

// 列上只有单据状态一栏：册上有作废留痕（依据与时刻成对），二态在库上完备。刻意没有「对账状态」与「结清状态」——
// 前者要由「有无未裁定异议」推，后者要把指向本单的核销汇总再下判，都是读面替人下结论；异议与后续账期纳入的条数
// 进技术详情，明细归详情面。

// —— 客户对账单签 ——

/** 对账单只带结算账户号；客户名要经已确认费用上登的「结算账户 → 结算相对方」再查参与方册。两跳都读不到就显账户号。 */
function statementCustomer(row: CustomerStatementRecord, accountOwners: Map<string, string>, names: PartyNames) {
  const party = accountOwners.get(row.account);
  const ref = party ? namedRef(names.party(party), party) : { primary: row.account };
  return <TwoLine primary={ref.primary} secondary={ref.secondary} />;
}

function statementTechnicalRows(row: CustomerStatementRecord) {
  return technicalRows([
    { label: '对账单号', value: row.statementNumber },
    { label: '结算账户', value: row.account },
    { label: '账期', value: row.period },
    { label: '费用行数', value: String(row.lineCount) },
    { label: '调整行数', value: String(row.adjustmentCount) },
    {
      label: '异议',
      value:
        row.disputes.length > 0
          ? row.disputes.map((d) => `${disputeLabel(d)}（${d.resolution ? labelFor(d.resolution, 'dispute') : '待裁定'}）`).join('；')
          : undefined,
    },
    {
      label: '后续纳入',
      value:
        row.subsequentInclusions.length > 0
          ? row.subsequentInclusions.map((i) => `${i.inclusion}（${labelFor(i.kind)}）`).join('；')
          : undefined,
    },
    { label: '作废依据', value: row.voidBasis },
    { label: '作废时间', value: row.voidedAt ? formatInstant(row.voidedAt) : undefined },
  ]);
}

function customerStatementColumns(accountOwners: Map<string, string>, names: PartyNames): ListColumn<CustomerStatementRecord>[] {
  return [
    {
      id: 'statement-number',
      header: '对账单号',
      className: 'whitespace-nowrap',
      render: (row) => <span className="font-mono type-body leading-4 text-idpxyz-textBright">{row.statementNumber}</span>,
    },
    { id: 'customer', header: '客户', className: 'whitespace-nowrap', render: (row) => statementCustomer(row, accountOwners, names) },
    { id: 'period', header: '账期', className: 'font-mono type-body leading-4', render: (row) => row.period },
    {
      id: 'total',
      header: '金额',
      align: 'right',
      className: 'font-mono type-body leading-4 tabular-nums',
      // 总额照实转写不由行数派生：总额严格等于所含明细之和是写口的不变量，读侧重算一遍只会在两处各说一套。
      render: (row) => formatMoney(row.totalAmount, row.currency),
    },
    {
      id: 'doc-state',
      header: '状态',
      align: 'center',
      // 二态由作废留痕渲染，不另造第二个布尔。作废不删行不改总额、替代单用新单号，所以「已作废」是留痕不是消失。
      // 「有异议待裁」只说单上挂着还没处理完的异议（未处理或待复核），不说对账成不成——那是读面替人下的结论。
      render: (row) =>
        row.voidBasis ? (
          <StatusPill label="已作废" tone="muted" />
        ) : openDisputesOf(row).length > 0 ? (
          <StatusPill label="有异议待裁" tone="warn" />
        ) : (
          <StatusPill code="PUBLISHED" />
        ),
    },
    {
      id: 'published-at',
      header: '发布时间',
      className: 'whitespace-nowrap font-mono type-body leading-4',
      render: (row) => formatInstant(row.publishedAt),
    },
    {
      id: 'technical',
      header: '',
      className: 'align-top',
      render: (row) => <TechnicalDetails rows={statementTechnicalRows(row)} className="max-w-md open:min-w-[18rem]" />,
    },
  ];
}

/** 两问一个取数函数：带 registry 的问对账单册，不带的问争议队列（list-conditions 的 statementListConditions）。 */
function loadStatements(
  conditions: CatalogueConditions,
  after: string | null,
): Promise<ApiResult<CustomerStatementListResponseBody | DisputedCustomerStatementListResponseBody>> {
  return isStatementRegistryConditions(conditions)
    ? listSettlementStatements(conditions, after)
    : listDisputedCustomerStatements(conditions, after);
}

// 「只看有未了结异议的」是按单上已记的异议筛（至少一项还没有终局结果），不是给单下对账状态：对账状态要连相对方确认一起判，
// 本产品不记相对方确认。张数取读口答的全部张数——在对账单册上自己数只数得到服务端交回的那一页。首页「今日待办」的待对账格点进来就落在这里。
function CustomerStatementsTable({
  names,
  disputedOnly,
  onDisputedOnlyChange,
}: {
  names: PartyNames;
  disputedOnly: boolean;
  onDisputedOnlyChange: (next: boolean) => void;
}) {
  const [search, setSearch] = useState('');
  const [refine, setRefine] = useState('');
  const [voidedHidden, setVoidedHidden] = useState(false);
  const conditions = statementListConditions(disputedOnly, useSettledKeyword(search), voidedHidden);
  const { answer, pagination, retry: retryStatements } = useCatalogueCursor(conditions, loadStatements);
  const [ownersKey, setOwnersKey] = useState(0);
  const [accountOwners, setAccountOwners] = useState<Map<string, string>>(() => new Map());
  const [selected, setSelected] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    // 账户归谁的对应只是名称转写的辅助一问：它不成也不影响对账单本身的呈现，列上退回显账户号。读口按页答，这里只取客户费用册的
    // 第一页，账户只登在页外费用上的单转写不出客户名。
    void listSettlementCharges(registryConditions('customer-charge')).then((next) => {
      if (!cancelled && next.kind === 'outcome') setAccountOwners(accountCounterparties(next.body.charges));
    });
    return () => {
      cancelled = true;
    };
  }, [disputedOnly, ownersKey]);

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const statements = body ? body.statements : [];
  const disputed = body && 'disputedCount' in body ? body : null;
  const visibleStatements = statements.filter((row) => matchesSearch(refine, [names.party(accountOwners.get(row.account))]));
  const retry = () => {
    retryStatements();
    setOwnersKey((value) => value + 1);
  };
  // 登记或处理之后重读这一签时列表先清空：右栏留着上一次读到的那张单，表单与刚才的答复不随之卸掉。
  const current = statements.find((row) => row.statementNumber === selected);
  const [pinned, setPinned] = useState<CustomerStatementRecord | undefined>(undefined);
  useEffect(() => {
    if (current) setPinned(current);
  }, [current]);
  const panelStatement = current ?? (pinned?.statementNumber === selected ? pinned : undefined);
  const panelGone = answer?.kind === 'outcome' && current === undefined && panelStatement !== undefined;

  return (
    <div className="flex flex-1 flex-col overflow-hidden lg:flex-row">
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-2 shrink-0">
          <label className="flex items-center gap-1.5 type-body leading-5">
            <input type="checkbox" checked={disputedOnly} onChange={(event) => onDisputedOnlyChange(event.target.checked)} />
            只看待对账
          </label>
          <span className="type-body leading-4 text-idpxyz-textMuted">
            {disputedOnly
              ? '只列有异议还没处理完（未处理或待复核）的单；客户确认没确认本系统不记，不在这里筛'
              : '勾上只看有异议还没处理完的单'}
          </span>
          <label className="flex items-center gap-1.5 type-body leading-5">
            <input type="checkbox" checked={voidedHidden} onChange={(event) => setVoidedHidden(event.target.checked)} />
            不看已作废的单
          </label>
        </div>
        <ListPageTemplate<CustomerStatementRecord>
          title={disputedOnly ? '待对账的客户对账单' : '客户对账单'}
          description={
            disputedOnly
              ? '单上至少一项异议还没有处理结果，或处理为待复核；接受、部分接受、拒绝都算处理完'
              : '已发给客户的对账单；有异议另记在单上，不改单的金额'
          }
          search={{ value: search, onChange: setSearch, placeholder: `搜对账单号、${disputeNumberName}、结算账户、账期` }}
          filters={<PageOnlySearch value={refine} onChange={setRefine} subject="客户名称" />}
          filterSummary={answer?.kind === 'outcome' ? statementSummary(statements, disputed?.disputedCount, voidedHidden) : undefined}
          columns={customerStatementColumns(accountOwners, names)}
          rows={visibleStatements}
          // 行键即对账单号：发布后固定，替代单用新单号。
          rowKey={(row) => row.statementNumber}
          onRowClick={(row) => setSelected(row.statementNumber)}
          emptyRowsNote={pageEmptyNote('对账单', statements.length)}
          pagination={pagination}
          viewState={catalogueViewState(answer, statements.length, retry, {
            module: info,
            ...(disputedOnly
              ? {
                  endpoint: 'GET /settlement-disputed-statements',
                  emptyTitle: '没有待对账的单',
                  emptyDescription: '已发布的对账单上没有还没处理完的异议。',
                }
              : {
                  endpoint: 'GET /settlement-statements?registry=customer-statement',
                  emptyTitle: '还没有发布过对账单',
                  emptyDescription: '费用确认后按账期归集成对账单，发布后在这里出现；页面不会预置对账单。',
                }),
            narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
          })}
        />
      </div>
      {/* 登记或处理落定后重读这一签：单上的争议与首页待对账的件数都从读口来，页面不自己往行上补。 */}
      <StatementDisputePanel statement={panelStatement} gone={panelGone} onAnswered={retry} />
    </div>
  );
}

/**
 * 对账单（UC-SA-003）。核销已按 UC-SA-005 口径独立成页（SettlementApplicationPage）——收付款事实与核销关系同对账单分别管理，
 * 收付款不改变对账单内容。本页不设发布或作废动作；右栏登记与处理客户争议（ADR-0354）。
 */
/** 「只看待对账」的地址同步：读自 hash、写回 hash（replaceState，不压后退记录、不触发壳层的 hashchange），做法同 useAddressKeyword。 */
function useDisputedOnlyPlace(): [boolean, (disputedOnly: boolean) => void] {
  const [disputedOnly, setDisputedOnly] = useState(() => disputedOnlyOf(window.location.hash));

  // 本页已开着时从首页点待对账格进来，地址变了而页不重挂载，要跟着地址走。
  useOwnAddressChange(() => setDisputedOnly(disputedOnlyOf(window.location.hash)));

  const place = (next: boolean) => {
    setDisputedOnly(next);
    const hash = window.location.hash;
    if (!hash.startsWith('#/')) return;
    window.history.replaceState(window.history.state, '', reconciliationHash(hash, next));
  };
  return [disputedOnly, place];
}

export function ReconciliationPage() {
  const names = usePartyNames();
  const [disputedOnly, place] = useDisputedOnlyPlace();
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="statements" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="statements">对账单</TabsTrigger>
          <TabsTrigger value="decisions">运营决定</TabsTrigger>
        </TabsList>
        <TabsContent value="statements" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomerStatementsTable names={names} disputedOnly={disputedOnly} onDisputedOnlyChange={place} />
        </TabsContent>
        <TabsContent value="decisions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          {/* 发布、作废与后续账期纳入：后端口都在，这里先用登记面板接上，照提示填 JSON 提交。 */}
          <MultiRegistrationPanel moduleId="reconciliation" targets={statementDecisionTargets} problemNote={decisionProblemNote} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
