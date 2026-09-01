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
import { chargeDirectionLabels, chargeStageLabels, labelOf, unregistered } from './presentation';

// unregisteredUntilConfirmed 是确认时固定那七项的统一呈现：未确认行整键不出现，如实写
// 「未登记」而不留白也不补默认值。七项共用一个渲染而不各写一遍——它们的缺席是同一件事
// （这一行还没确认），写七遍就会有一处日后跟别处不一样。
function unregisteredUntilConfirmed(value: string | undefined) {
  return value ?? <span className="text-idpxyz-textMuted">{unregistered}</span>;
}

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['charges-billing'];

// 本页两签，都接 GET /settlement-charges 按 registry 分派。
//
// 两册各自成形而不合流成一张「费用明细」表（票 admin-skeleton-closure-batch/04 对栏裁定）：
// 客户费用有阶段与确认依据、无采购规则版本，供应商预期成本有版本链与纠错原因、无阶段。
// 合流要现编一个「收付方向」——库上客户费用册不存这一维，而 CONTEXT 明写确认费用必须固定
// 收付方向且「不能通过当前组织、当前客户属性或报表筛选临时推断」，由读面按行落在哪张表反推
// 正是这条禁的东西。
//
// 旧骨架十二栏里的版本与计费重量采用两栏仍撤：它们是**册级缺席**（这一册根本不记这件事），
// 整列永远「未登记」不叫如实，那会把「本册不记」说成「本册记漏了」，反过来招人去别处推断
// 补齐。版本这一栏尤其不该回来——ADR-0087 决定二给客户侧选的是调整明细册而不是版本链，
// customer_charge 主键仍是一费用一行；调整落 charge_adjustment，要显示「被调整过」是另一
// 条读面，不是把版本栏加回来。
//
// 主要计费范围、收付方向、责任法人、结算相对方四栏**已随 ADR-0087 决定一加回**，另加合同
// 或责任依据与来源事实两栏：那七项现在真在册上（迁移 0014），缺席从册级降成了行级——未确认
// 行整键不出现，由库上同在或同缺的 CHECK 守着。撤栏的判据没变，是判据的输入变了。

// 金额与币种同格呈现不拆两栏：原币、合同结算币与换算依据是从同一个评价采用来的一组，
// CONTEXT 要求整组重述不拆散。金额是币种最小单位的十进制计数串，照实转写不做换算。
function amountWithCurrency(amount: string, currency: string): string {
  return `${amount} ${currency}`;
}

// —— 客户费用签 ——

const customerChargeColumns: ListColumn<CustomerChargeRecord>[] = [
  { id: 'charge', header: '费用明细标识', className: 'font-mono text-xs', render: (row) => row.charge },
  { id: 'fee-item', header: '费用项目', className: 'font-mono text-xs', render: (row) => row.feeItem },
  {
    id: 'stage',
    header: '阶段',
    align: 'center',
    className: 'w-[64px]',
    // 封闭三格，与库上一致。旧骨架注释写的「预估/暂估/确认/调整」四格不成立：调整是追加的
    // 调整明细（各由唯一创建用例形成、另落自己的册），不是阶段的第四个取值。
    render: (row) => labelOf(chargeStageLabels, row.stage),
  },
  {
    id: 'original',
    header: '原币金额',
    align: 'right',
    className: 'font-mono text-xs',
    render: (row) => amountWithCurrency(row.originalAmount, row.originalCurrency),
  },
  {
    id: 'settlement',
    header: '合同结算币金额',
    align: 'right',
    className: 'font-mono text-xs',
    render: (row) => amountWithCurrency(row.settlementAmount, row.settlementCurrency),
  },
  {
    id: 'conversion',
    header: '换算依据',
    className: 'font-mono text-xs',
    // 缺席不是缺数据而是「这一步不存在」的正面形状（原币即结算币时无换算），故不写「未登记」。
    render: (row) => row.conversionStep ?? <span className="text-idpxyz-textMuted">无换算</span>,
  },
  { id: 'evaluation', header: '评价引用', className: 'font-mono text-xs', render: (row) => row.evaluation },
  {
    id: 'confirmation-basis',
    header: '确认依据',
    className: 'font-mono text-xs',
    // 真正的行级缺席：预估与暂估行没有确认依据（库上守着依据与确认时刻两半同在或同缺）。
    // 空即「尚未确认」，读的人据它去催确认。
    render: (row) =>
      row.confirmationBasis ?? <span className="text-idpxyz-textMuted">{unregistered}</span>,
  },
  // 确认时固定的七项（ADR-0087 决定一）。这六栏是票 04 对栏裁定当初以**册级缺席**撤下的
  // 那几栏加回来的——判据没变，是判据的输入变了：册上真有了这些列，缺席从册级降成了行级
  // （未确认行整键不出现，那是库上同在或同缺 CHECK 的正面结果），按同一条判据就该保留栏
  // 并如实显示「未登记」。
  //
  // 「来源事实」不另设栏而与评价引用相邻同格：它是评价的输入，两者是一条链上的两环，
  // 分成互不相邻的两栏会让读者以为可以各取各的。
  {
    id: 'responsible-entity',
    header: '责任法人',
    className: 'font-mono text-xs',
    render: (row) => unregisteredUntilConfirmed(row.responsibleEntity),
  },
  {
    id: 'counterparty',
    header: '结算相对方',
    className: 'font-mono text-xs',
    render: (row) => unregisteredUntilConfirmed(row.counterparty),
  },
  {
    id: 'charge-direction',
    header: '收付方向',
    // 照册转写，**不由页面推断**：CONTEXT 明写这一项「不能通过当前组织、当前客户属性或
    // 报表筛选临时推断」。集外取值原样回显，不译成像样的话。
    render: (row) =>
      row.chargeDirection
        ? labelOf(chargeDirectionLabels, row.chargeDirection)
        : unregisteredUntilConfirmed(undefined),
  },
  {
    id: 'settlement-account',
    header: '结算账户',
    className: 'font-mono text-xs',
    render: (row) => unregisteredUntilConfirmed(row.settlementAccount),
  },
  {
    id: 'contract-basis',
    header: '合同或责任依据',
    className: 'font-mono text-xs',
    // 与「确认依据」是两栏不是一栏：后者说的是哪份依据让它可以确认，这一栏说的是这笔钱
    // 依据哪份合同该收付，合用会让其中一个永远说不出口。
    render: (row) => unregisteredUntilConfirmed(row.contractBasis),
  },
  {
    id: 'primary-charging-scope',
    header: '主要计费范围',
    className: 'font-mono text-xs',
    render: (row) => unregisteredUntilConfirmed(row.primaryChargingScope),
  },
  {
    id: 'source-fact',
    header: '来源事实',
    className: 'font-mono text-xs',
    // 与评价引用分两栏：评价是依据，来源事实是评价的输入。
    render: (row) => unregisteredUntilConfirmed(row.sourceFact),
  },
  {
    id: 'required-basis-kind',
    header: '要求依据种类',
    className: 'font-mono text-xs',
    // 与「已到达确认依据」分两格上列，**不代算交集**：「这类费用要哪一种」与「到了哪几种」
    // 是两张表两个答案，分两张表正是为了让「确认条件已满足」没有第三条成立路径。
    // 缺席表示该费用项目在确认条件目录里没有行——与「配了但依据没到」续办不同：前者要人去
    // 配条件，后者要人去催依据，故两格分开由读者判。
    render: (row) =>
      row.requiredBasisKind ?? <span className="text-idpxyz-textMuted">未配置确认条件</span>,
  },
  {
    id: 'arrived-bases',
    header: '已到达确认依据',
    render: (row) =>
      row.confirmationBases.length > 0 ? (
        <div className="min-w-44 font-mono text-xs">
          {row.confirmationBases.map((basis) => (
            <p key={`${basis.basisKind}:${basis.basis}`}>
              {basis.basisKind}／{basis.basis}（{formatInstant(basis.recordedAt)}）
            </p>
          ))}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">尚无</span>
      ),
  },
  {
    id: 'formed-at',
    header: '形成时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.formedAt),
  },
  {
    id: 'confirmed-at',
    header: '确认时间',
    className: 'min-w-44 font-mono text-xs',
    // 与确认依据成对：未确认时服务端整键不出现，不为它编造零时刻。
    render: (row) =>
      row.confirmedAt ? (
        formatInstant(row.confirmedAt)
      ) : (
        <span className="text-idpxyz-textMuted">{unregistered}</span>
      ),
  },
];

function CustomerChargesTable() {
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
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。
  const visibleCharges = needle
    ? charges.filter((row) =>
        [row.charge, row.feeItem, row.evaluation].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : charges;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CustomerChargeRecord>
      title="客户费用"
      description={`${info.owner}——一行即一条客户费用；收付方向、责任法人与结算相对方不在本册，页面不推断`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索费用明细标识 / 费用项目 / 评价引用',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 条」会与状态区
        // 「这不是登记册为空」直接矛盾。
        answer?.kind === 'outcome' ? `当前返回 ${charges.length} 条客户费用` : undefined
      }
      columns={customerChargeColumns}
      rows={visibleCharges}
      // 行键循库主键（租户，费用）：一费用一行，册上没有版本列。
      rowKey={(row) => row.charge}
      viewState={catalogueViewState(answer, charges.length, retry, {
        module: info,
        endpoint: 'GET /settlement-charges?registry=customer-charge',
        emptyTitle: '当前租户尚无已登记的客户费用',
        emptyDescription:
          '读取入口已配置，但客户费用登记册为空；费用由计价评价经 UC-SA-002 形成，事务链在接入渠道墙后面，页面不会预置费用。',
      })}
    />
  );
}

// —— 供应商预期成本签 ——

// 本册不设阶段栏：整册属预估口径（CONTEXT「预期成本属预估口径，从不进对账单」），那是
// **册级事实**。补一个恒为「预估」的阶段栏，是把册级事实伪装成行级取值，还会让读者以为
// 它可能变成别的值。该册另有客户册没有的栏（采购规则版本、供应商协议、运输收费发生项与
// 版本、前版与纠错原因），按册上原样呈现——这正是两册不合流的用处。
const supplierExpectedCostColumns: ListColumn<SupplierExpectedCostRecord>[] = [
  { id: 'version', header: '成本版本', className: 'font-mono text-xs', render: (row) => row.version },
  { id: 'fee-item', header: '费用项目', className: 'font-mono text-xs', render: (row) => row.feeItem },
  { id: 'occurrence', header: '运输收费发生项', className: 'font-mono text-xs', render: (row) => row.occurrence },
  { id: 'occurrence-reason', header: '发生原因', className: 'font-mono text-xs', render: (row) => row.occurrenceReason },
  {
    id: 'occurrence-version',
    header: '发生项版本',
    className: 'font-mono text-xs',
    render: (row) => row.occurrenceVersion,
  },
  {
    id: 'occurred-at',
    header: '发生时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.occurredAt),
  },
  {
    id: 'purchase-rule-version',
    header: '采购规则版本',
    className: 'font-mono text-xs',
    render: (row) => row.purchaseRuleVersion,
  },
  { id: 'agreement', header: '供应商协议', className: 'font-mono text-xs', render: (row) => row.agreement },
  { id: 'evaluation', header: '评价引用', className: 'font-mono text-xs', render: (row) => row.evaluation },
  {
    id: 'original',
    header: '原币金额',
    align: 'right',
    className: 'font-mono text-xs',
    render: (row) => amountWithCurrency(row.originalAmount, row.originalCurrency),
  },
  {
    id: 'settlement',
    header: '结算币金额',
    align: 'right',
    className: 'font-mono text-xs',
    render: (row) => amountWithCurrency(row.settlementAmount, row.settlementCurrency),
  },
  {
    id: 'conversion',
    header: '换算依据',
    className: 'font-mono text-xs',
    render: (row) => row.conversionStep ?? <span className="text-idpxyz-textMuted">无换算</span>,
  },
  {
    id: 'prior-version',
    header: '前版',
    className: 'font-mono text-xs',
    // 与纠错原因成对缺席表示首版——计价纠错换版本、原版本保留，一份成本的历史因此是多行
    // 而不是一行被改写。故缺席不是「未登记」。
    render: (row) => row.priorVersion ?? <span className="text-idpxyz-textMuted">首版</span>,
  },
  {
    id: 'correction-reason',
    header: '纠错原因',
    render: (row) => row.correctionReason ?? <span className="text-idpxyz-textMuted">—</span>,
  },
  {
    id: 'recorded-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.recordedAt),
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
        [row.version, row.feeItem, row.occurrence, row.agreement].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : costs;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<SupplierExpectedCostRecord>
      title="供应商预期成本"
      description={`${info.owner}——整册属预估口径、从不进对账单，故不设阶段栏；一行即一版，纠错换版本不覆盖历史`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索成本版本 / 费用项目 / 收费发生项 / 供应商协议',
      }}
      filterSummary={
        answer?.kind === 'outcome' ? `当前返回 ${costs.length} 版预期成本` : undefined
      }
      columns={supplierExpectedCostColumns}
      rows={visibleCosts}
      // 行键即版本标识：一版一行，历史版本连同当前版一并在册。
      rowKey={(row) => row.version}
      viewState={catalogueViewState(answer, costs.length, retry, {
        module: info,
        endpoint: 'GET /settlement-charges?registry=supplier-expected-cost',
        emptyTitle: '当前租户尚无已登记的供应商预期成本',
        emptyDescription:
          '读取入口已配置，但供应商预期成本登记册为空；预期成本由运输收费发生项经采购规则评价形成，事务链在接入渠道墙后面，页面不会预置成本。',
      })}
    />
  );
}

/**
 * 费用与计费（settlement-accounting）。两册各自成签：客户费用（UC-SA-002 形成，经历预估、
 * 暂估与确认三段）与供应商预期成本（预估口径的版本册）。
 *
 * 本页是查阅面，不设任何调整创建动作——每类调整只能由 CONTEXT「调整类型与唯一所有权」表
 * 指定的唯一创建用例形成，接线不取得创建权。
 */
export function ChargesBillingPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="customer" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="customer">客户费用</TabsTrigger>
          <TabsTrigger value="supplier">供应商预期成本</TabsTrigger>
        </TabsList>
        <TabsContent value="customer" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomerChargesTable />
        </TabsContent>
        <TabsContent value="supplier" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <SupplierExpectedCostsTable />
        </TabsContent>
      </Tabs>
    </div>
  );
}
