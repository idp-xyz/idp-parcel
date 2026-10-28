import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listSettlementCharges,
  type CustomerChargeListResponseBody,
  type CustomerChargeRecord,
  type SupplierExpectedCostListResponseBody,
  type SupplierExpectedCostRecord,
} from './api';
import { labelFor } from '../../vocabulary/status';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { AmountCell, TwoLine, technicalRows } from './ops-ui';
import { feeItemLabel, parcelFromRef, sourceFactLabel } from './ops-refs';
import { namedRef, usePartyNames, type PartyNames } from './party-names';

// 费用与计费页（主责上下文 settlement-accounting：费用项目、费用明细与费用调整的版本与依据）。
// 页头与列只说运营的话；上下文名、用例号与各种引用都留在本文件的注释和每行的「技术详情」折叠里。
const info = moduleInfoById['charges-billing'];

// 两册各自成签而不合流成一张「费用明细」表：客户费用有阶段与确认依据、无价卡版本，供应商预期成本有版本链与纠错原因、
// 无阶段。合流要现编一个「收付方向」——客户费用册不存这一维，而 CONTEXT 明写确认费用必须固定收付方向且「不能通过
// 当前组织、当前客户属性或报表筛选临时推断」，由读面按行落在哪张表反推正是这条禁的东西。
//
// 确认时固定的七项（责任法人、结算相对方、收付方向、结算账户、合同或责任依据、主要计费范围、来源事实，ADR-0087 决定一）
// 未确认行整键不出现，那是「这一行还没确认」的正面形状；页面不为它们补默认值、不由别处推断。主列里只拿其中两项给人看
// （结算相对方转写成客户名、来源事实解析成运单号），其余进技术详情。

// —— 客户费用签 ——

/** 运单号从来源事实或确认依据里认（两者都长成 <族>/<包裹号>/…）；都认不出就显费用来源的中文说法。 */
function parcelCell(row: CustomerChargeRecord) {
  const parcel = parcelFromRef(row.sourceFact) ?? parcelFromRef(row.confirmationBasis);
  if (parcel) return <span className="font-mono text-xs text-idpxyz-textBright">{parcel}</span>;
  const family = sourceFactLabel(row.sourceFact);
  if (family) return <span className="text-idpxyz-text">{family}</span>;
  // 未确认行上来源事实整键不出现：运单在确认时才随来源事实固定到这一行，不是漏登。
  return (
    <span className="text-idpxyz-textMuted" title="来源事实在费用确认时固定到这一行">
      确认后关联
    </span>
  );
}

function customerCell(row: CustomerChargeRecord, names: PartyNames) {
  if (!row.counterparty) {
    return (
      <span className="text-idpxyz-textMuted" title="结算相对方在费用确认时固定到这一行">
        确认后固定
      </span>
    );
  }
  const ref = namedRef(names.party(row.counterparty), row.counterparty);
  return <TwoLine primary={ref.primary} secondary={ref.secondary} />;
}

/** 技术详情里的行：标识、依据、评价、账户、合同、范围一类排查与对账才要的字。 */
function customerChargeTechnicalRows(row: CustomerChargeRecord) {
  return technicalRows([
    { label: '费用明细', value: row.charge },
    { label: '费用项目码', value: row.feeItem },
    { label: '计费记录', value: row.evaluation },
    { label: '来源事实', value: row.sourceFact },
    { label: '确认依据', value: row.confirmationBasis },
    { label: '要求依据', value: row.requiredBasisKind ? labelFor(row.requiredBasisKind) : '未配置确认条件' },
    {
      label: '已到依据',
      value: row.confirmationBases
        .map((basis) => `${labelFor(basis.basisKind)} ${basis.basis}（${formatInstant(basis.recordedAt)}）`)
        .join('；'),
    },
    { label: '收付方向', value: row.chargeDirection ? labelFor(row.chargeDirection) : undefined },
    { label: '公司主体', value: row.responsibleEntity },
    { label: '结算账户', value: row.settlementAccount },
    { label: '合同依据', value: row.contractBasis },
    { label: '计费范围', value: row.primaryChargingScope },
    { label: '换算依据', value: row.conversionStep },
    { label: '形成时间', value: formatInstant(row.formedAt) },
  ]);
}

function customerChargeColumns(names: PartyNames): ListColumn<CustomerChargeRecord>[] {
  return [
    { id: 'parcel', header: '运单号', render: parcelCell },
    { id: 'customer', header: '客户', render: (row) => customerCell(row, names) },
    { id: 'fee-item', header: '费用项目', render: (row) => feeItemLabel(row.feeItem) },
    {
      id: 'amount',
      header: '金额',
      align: 'right',
      // 原币与结算币同格两行，不拆两栏：它们是从同一个评价采用来的一组，CONTEXT 要求整组重述不拆散。
      render: (row) => (
        <AmountCell
          originalAmount={row.originalAmount}
          originalCurrency={row.originalCurrency}
          settlementAmount={row.settlementAmount}
          settlementCurrency={row.settlementCurrency}
        />
      ),
    },
    {
      id: 'stage',
      header: '阶段',
      align: 'center',
      // 封闭三格，与库上一致；「调整」不是第四格——调整是追加的调整明细，各由唯一创建用例形成、另落自己的册。
      render: (row) => <StatusPill code={row.stage} />,
    },
    {
      id: 'confirmed-at',
      header: '确认时间',
      className: 'whitespace-nowrap font-mono text-xs',
      // 与确认依据成对：未确认时整键不出现，不为它编造零时刻。
      render: (row) => (row.confirmedAt ? formatInstant(row.confirmedAt) : <span className="text-idpxyz-textMuted">未确认</span>),
    },
    {
      id: 'technical',
      header: '',
      className: 'w-[1%] whitespace-nowrap align-top',
      render: (row) => <TechnicalDetails rows={customerChargeTechnicalRows(row)} />,
    },
  ];
}

function CustomerChargesTable({ names }: { names: PartyNames }) {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<CustomerChargeListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSettlementCharges('customer-charge').then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const charges = answer?.kind === 'outcome' ? answer.body.charges : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。检索的词包括转写出来的客户名与运单号。
  const visibleCharges = needle
    ? charges.filter((row) =>
        [
          row.charge,
          row.feeItem,
          feeItemLabel(row.feeItem),
          row.evaluation,
          row.counterparty,
          names.party(row.counterparty),
          parcelFromRef(row.sourceFact) ?? parcelFromRef(row.confirmationBasis),
        ].some((value) => value?.toLowerCase().includes(needle)),
      )
    : charges;
  const retry = () => setReloadKey((value) => value + 1);
  const confirmed = charges.filter((row) => row.stage === 'CONFIRMED').length;

  return (
    <ListPageTemplate<CustomerChargeRecord>
      title="客户费用"
      description="每一票运单该向客户收的费用；确认后才会进对账单"
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜运单号、客户、费用项目',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 条」会与状态区「这不是登记册为空」直接矛盾。
        answer?.kind === 'outcome' ? `${charges.length} 笔 · 已确认 ${confirmed} 笔 · 待确认 ${charges.length - confirmed} 笔` : undefined
      }
      columns={customerChargeColumns(names)}
      rows={visibleCharges}
      // 行键循库主键（租户，费用）：一费用一行，册上没有版本列。
      rowKey={(row) => row.charge}
      emptyRowsNote="没有匹配的费用"
      viewState={catalogueViewState(answer, charges.length, retry, {
        module: info,
        endpoint: 'GET /settlement-charges?registry=customer-charge',
        emptyTitle: '还没有客户费用',
        emptyDescription: '运单收寄、签收之后系统会按价格表自动算出费用，这里会随之出现；页面不会预置任何费用。',
      })}
    />
  );
}

// —— 供应商预期成本签 ——

// 本册不设阶段栏：整册属预估口径（CONTEXT「预期成本属预估口径，从不进对账单」），那是册级事实。补一个恒为「预估」的
// 阶段栏是把册级事实伪装成行级取值。版本现况（首版 / 纠错版）是行级的，才上列。

function supplierCostTechnicalRows(row: SupplierExpectedCostRecord) {
  return technicalRows([
    { label: '成本版本', value: row.version },
    { label: '费用项目码', value: row.feeItem },
    { label: '收费发生项', value: `${row.occurrence}（${row.occurrenceVersion}）` },
    { label: '价格表版本', value: row.purchaseRuleVersion },
    { label: '供应商协议', value: row.agreement },
    { label: '计费记录', value: row.evaluation },
    { label: '换算依据', value: row.conversionStep },
    { label: '前一版', value: row.priorVersion },
    { label: '纠错原因', value: row.correctionReason },
    { label: '登记时间', value: formatInstant(row.recordedAt) },
  ]);
}

const supplierExpectedCostColumns: ListColumn<SupplierExpectedCostRecord>[] = [
  {
    id: 'agreement',
    header: '供应商协议',
    // 本册不带供应商参与方编码，只带协议引用；协议号就是运营认供应商的字，原样作主字。
    render: (row) => <span className="font-mono text-xs text-idpxyz-textBright">{row.agreement}</span>,
  },
  { id: 'fee-item', header: '费用项目', render: (row) => feeItemLabel(row.feeItem) },
  {
    id: 'reason',
    header: '发生原因',
    render: (row) => labelFor(row.occurrenceReason),
  },
  {
    id: 'amount',
    header: '金额',
    align: 'right',
    render: (row) => (
      <AmountCell
        originalAmount={row.originalAmount}
        originalCurrency={row.originalCurrency}
        settlementAmount={row.settlementAmount}
        settlementCurrency={row.settlementCurrency}
      />
    ),
  },
  {
    id: 'version-state',
    header: '版本',
    align: 'center',
    // 前版与纠错原因成对缺席表示首版——计价纠错换版本、原版本保留，一份成本的历史因此是多行而不是一行被改写。
    render: (row) => (row.priorVersion ? <StatusPill code="CORRECTION" label="纠错版" /> : <StatusPill label="首版" tone="muted" />),
  },
  {
    id: 'occurred-at',
    header: '发生时间',
    className: 'whitespace-nowrap font-mono text-xs',
    render: (row) => formatInstant(row.occurredAt),
  },
  {
    id: 'technical',
    header: '',
    className: 'w-[1%] whitespace-nowrap align-top',
    render: (row) => <TechnicalDetails rows={supplierCostTechnicalRows(row)} />,
  },
];

function SupplierExpectedCostsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<SupplierExpectedCostListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSettlementCharges('supplier-expected-cost').then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const costs = answer?.kind === 'outcome' ? answer.body.costs : [];
  const needle = search.trim().toLowerCase();
  const visibleCosts = needle
    ? costs.filter((row) =>
        [row.version, row.feeItem, feeItemLabel(row.feeItem), row.occurrence, row.agreement].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : costs;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<SupplierExpectedCostRecord>
      title="供应商预期成本"
      description="按供应商协议预估的承运成本；只作预估，不进对账单，纠错时另起一版"
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜供应商协议、费用项目',
      }}
      filterSummary={answer?.kind === 'outcome' ? `${costs.length} 版预期成本` : undefined}
      columns={supplierExpectedCostColumns}
      rows={visibleCosts}
      // 行键即版本标识：一版一行，历史版本连同当前版一并在册。
      rowKey={(row) => row.version}
      emptyRowsNote="没有匹配的成本"
      viewState={catalogueViewState(answer, costs.length, retry, {
        module: info,
        endpoint: 'GET /settlement-charges?registry=supplier-expected-cost',
        emptyTitle: '还没有供应商预期成本',
        emptyDescription: '运输交接登记之后系统会按供应商协议算出预期成本，这里会随之出现；页面不会预置任何成本。',
      })}
    />
  );
}

/**
 * 费用与计费。两册各自成签：客户费用（UC-SA-002 形成，经历预估、暂估与确认三段）与供应商预期成本（预估口径的版本册）。
 * 本页是查阅面，不设任何调整创建动作——每类调整只能由 CONTEXT「调整类型与唯一所有权」表指定的唯一创建用例形成。
 */
export function ChargesBillingPage() {
  // 参与方名称一问放在页级，两签共用，切签不重问。
  const names = usePartyNames();
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="customer" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="customer">客户费用</TabsTrigger>
          <TabsTrigger value="supplier">供应商预期成本</TabsTrigger>
        </TabsList>
        <TabsContent value="customer" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomerChargesTable names={names} />
        </TabsContent>
        <TabsContent value="supplier" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <SupplierExpectedCostsTable />
        </TabsContent>
      </Tabs>
    </div>
  );
}
