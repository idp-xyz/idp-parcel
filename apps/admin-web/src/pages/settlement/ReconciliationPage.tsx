import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listSettlementCharges,
  listSettlementStatements,
  type CustomerStatementListResponseBody,
  type CustomerStatementRecord,
  type SupplierBillReceptionListResponseBody,
  type SupplierBillReceptionRecord,
} from './api';
import { TwoLine, formatMoney, technicalRows } from './ops-ui';
import { accountCounterparties } from './ops-refs';
import { namedRef, usePartyNames, type PartyNames } from './party-names';

// 对账单页（主责上下文 settlement-accounting「结算账户、对账与争议」）。两签都接 GET /settlement-statements 按 registry 分派。
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
          ? row.disputes.map((d) => `${d.dispute}（${d.resolution ? labelFor(d.resolution, 'dispute') : '待裁定'}）`).join('；')
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
      render: (row) => <span className="font-mono text-xs text-idpxyz-textBright">{row.statementNumber}</span>,
    },
    { id: 'customer', header: '客户', className: 'whitespace-nowrap', render: (row) => statementCustomer(row, accountOwners, names) },
    { id: 'period', header: '账期', className: 'font-mono text-xs', render: (row) => row.period },
    {
      id: 'total',
      header: '金额',
      align: 'right',
      className: 'font-mono text-xs tabular-nums',
      // 总额照实转写不由行数派生：总额严格等于所含明细之和是写口的不变量，读侧重算一遍只会在两处各说一套。
      render: (row) => formatMoney(row.totalAmount, row.currency),
    },
    {
      id: 'doc-state',
      header: '状态',
      align: 'center',
      // 二态由作废留痕渲染，不另造第二个布尔。作废不删行不改总额、替代单用新单号，所以「已作废」是留痕不是消失。
      // 「有异议待裁」只说单上挂着没裁的异议，不说对账成不成——那是读面替人下的结论。
      render: (row) =>
        row.voidBasis ? (
          <StatusPill label="已作废" tone="muted" />
        ) : row.disputes.some((dispute) => !dispute.resolution) ? (
          <StatusPill label="有异议待裁" tone="warn" />
        ) : (
          <StatusPill code="PUBLISHED" />
        ),
    },
    {
      id: 'published-at',
      header: '发布时间',
      className: 'whitespace-nowrap font-mono text-xs',
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

function CustomerStatementsTable({ names }: { names: PartyNames }) {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<CustomerStatementListResponseBody> | null>(null);
  const [accountOwners, setAccountOwners] = useState<Map<string, string>>(() => new Map());

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSettlementStatements('customer-statement').then((next) => {
      if (!cancelled) setAnswer(next);
    });
    // 账户归谁的对应只是名称转写的辅助一问：它不成也不影响对账单本身的呈现，列上退回显账户号。
    void listSettlementCharges('customer-charge').then((next) => {
      if (!cancelled && next.kind === 'outcome') setAccountOwners(accountCounterparties(next.body.charges));
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const statements = answer?.kind === 'outcome' ? answer.body.statements : [];
  const needle = search.trim().toLowerCase();
  const visibleStatements = needle
    ? statements.filter((row) =>
        [row.statementNumber, row.account, row.period, names.party(accountOwners.get(row.account))].some((value) =>
          value?.toLowerCase().includes(needle),
        ),
      )
    : statements;
  const retry = () => setReloadKey((value) => value + 1);
  const voided = statements.filter((row) => row.voidBasis).length;

  return (
    <ListPageTemplate<CustomerStatementRecord>
      title="客户对账单"
      description="已发给客户的对账单；有异议另记在单上，不改单的金额"
      search={{ value: search, onChange: setSearch, placeholder: '搜对账单号、客户、账期' }}
      filterSummary={answer?.kind === 'outcome' ? `${statements.length} 张 · 已作废 ${voided} 张` : undefined}
      columns={customerStatementColumns(accountOwners, names)}
      rows={visibleStatements}
      // 行键即对账单号：发布后固定，替代单用新单号。
      rowKey={(row) => row.statementNumber}
      emptyRowsNote="没有匹配的对账单"
      viewState={catalogueViewState(answer, statements.length, retry, {
        module: info,
        endpoint: 'GET /settlement-statements?registry=customer-statement',
        emptyTitle: '还没有发布过对账单',
        emptyDescription: '费用确认后按账期归集成对账单，发布后在这里出现；页面不会预置对账单。',
      })}
    />
  );
}

// —— 供应商账单签 ——

function receptionTechnicalRows(row: SupplierBillReceptionRecord) {
  return technicalRows([
    { label: '账单主张', value: row.claim },
    { label: '主张版本', value: row.claimVersion },
    { label: '供应商编码', value: row.supplier },
    { label: '公司主体', value: row.legalEntity },
    { label: '账期', value: row.period },
    { label: '币种', value: row.currency },
    { label: '登记时间', value: formatInstant(row.recordedAt) },
  ]);
}

function supplierBillColumns(names: PartyNames): ListColumn<SupplierBillReceptionRecord>[] {
  return [
    {
      id: 'claim',
      header: '账单号',
      render: (row) => <TwoLine primary={row.claim} secondary={row.claimVersion} mono />,
    },
    {
      id: 'supplier',
      header: '供应商',
      render: (row) => {
        const ref = namedRef(names.party(row.supplier), row.supplier);
        return <TwoLine primary={ref.primary} secondary={ref.secondary} />;
      },
    },
    { id: 'period', header: '账期', className: 'font-mono text-xs', render: (row) => row.period },
    {
      id: 'lines',
      header: '明细',
      align: 'right',
      className: 'whitespace-nowrap text-xs',
      // 本册没有总额，只有行数：明细行数与已对上的行数并列而不折成「匹配率」——未匹配、部分匹配与已匹配是三种结果。
      render: (row) => `${row.lineCount} 行 · 已对上 ${row.matchCount} 行（${row.currency}）`,
    },
    {
      id: 'audit',
      header: '状态',
      align: 'center',
      // 照实转写审核授权有没有配，不折成「已审核」：授权未配置时审核停在未决，不默认放行也不虚构审核人。
      render: (row) =>
        row.auditAuthorityConfigured ? <StatusPill label="可审核" tone="info" /> : <StatusPill label="审核人未配置" tone="warn" />,
    },
    {
      id: 'recorded-at',
      header: '收到时间',
      className: 'whitespace-nowrap font-mono text-xs',
      render: (row) => formatInstant(row.recordedAt),
    },
    {
      id: 'technical',
      header: '',
      className: 'align-top',
      render: (row) => <TechnicalDetails rows={receptionTechnicalRows(row)} className="max-w-md open:min-w-[18rem]" />,
    },
  ];
}

function SupplierBillReceptionsTable({ names }: { names: PartyNames }) {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<SupplierBillReceptionListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSettlementStatements('supplier-bill-reception').then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const receptions = answer?.kind === 'outcome' ? answer.body.receptions : [];
  const needle = search.trim().toLowerCase();
  const visibleReceptions = needle
    ? receptions.filter((row) =>
        [row.claim, row.supplier, names.party(row.supplier), row.legalEntity, row.period].some((value) =>
          value?.toLowerCase().includes(needle),
        ),
      )
    : receptions;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<SupplierBillReceptionRecord>
      title="供应商账单"
      description="供应商发来的账单；审核通过的部分才算应付，有争议或被拒的金额不算"
      search={{ value: search, onChange: setSearch, placeholder: '搜账单号、供应商、账期' }}
      filterSummary={answer?.kind === 'outcome' ? `${receptions.length} 份` : undefined}
      columns={supplierBillColumns(names)}
      rows={visibleReceptions}
      // 行键循主张加版本：主张换版本另成一行，原版本保留。
      rowKey={(row) => `${row.claim}:${row.claimVersion}`}
      emptyRowsNote="没有匹配的账单"
      viewState={catalogueViewState(answer, receptions.length, retry, {
        module: info,
        endpoint: 'GET /settlement-statements?registry=supplier-bill-reception',
        emptyTitle: '还没有收到供应商账单',
        emptyDescription: '供应商账单经接口或人工接收后在这里出现；页面不会预置账单。',
      })}
    />
  );
}

/**
 * 对账单。本页承载对账单视角的两册：客户对账单（UC-SA-003）与供应商账单主张（UC-SA-004）。核销已按 UC-SA-005 口径
 * 独立成页（SettlementApplicationPage）——收付款事实与核销关系同对账单分别管理，收付款不改变对账单内容。
 * 本页是查阅面，不设发布、作废或异议裁定动作——那些各由自己的唯一创建用例形成。
 */
export function ReconciliationPage() {
  const names = usePartyNames();
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="customer" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="customer">客户对账单</TabsTrigger>
          <TabsTrigger value="supplier">供应商账单</TabsTrigger>
        </TabsList>
        <TabsContent value="customer" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomerStatementsTable names={names} />
        </TabsContent>
        <TabsContent value="supplier" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <SupplierBillReceptionsTable names={names} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
