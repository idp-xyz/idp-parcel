import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listSettlementStatements,
  type CustomerStatementListResponseBody,
  type CustomerStatementRecord,
  type SupplierBillReceptionListResponseBody,
  type SupplierBillReceptionRecord,
} from '../settlement/api';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['reconciliation'];

// 本页两签，都接 GET /settlement-statements 按 registry 分派。读面类型取自 pages/settlement/api
// ——本页与收付款核销页历史上落在 pages/governance/，但主责上下文是 settlement-accounting，
// 四页镜像的是同一个 Go 包；类型在两处各存一份就会有两处各自维护同一批字段。
//
// 旧骨架三个状态栏随对栏裁定分别处置，不可一并处理：
//   单据状态——**保留**。册上有作废留痕（依据与时刻成对约束守着），二态在库上完备：有留痕即
//     已作废，无即已发布。这是同一事实换个词，不是新判断。
//   对账状态——**撤栏**。册级缺席。册上有异议，但异议不是对账状态：没有异议不等于相对方已对账
//     确认。由「有无未裁定异议」推出对账状态是读面替人下结论，会把「还没人看」说成「已对账」。
//   结清状态——**撤栏**。册级缺席且跨册。结清要把指向本单的核销汇总，再对「全额／部分／未结」
//     下判；汇总可以，下判超出「派生只到求和与计数为止」这条线。
//
// 新增费用行数与调整行数两栏（读面已供）：行数分得开「这张单里有几行」与「一行都没有」。
// 异议与后续账期纳入不进列表，按详情面处置。

// —— 客户对账单签 ——

const customerStatementColumns: ListColumn<CustomerStatementRecord>[] = [
  {
    id: 'statement-number',
    header: '对账单号',
    className: 'font-mono text-xs',
    render: (row) => row.statementNumber,
  },
  { id: 'account', header: '结算账户', className: 'font-mono text-xs', render: (row) => row.account },
  { id: 'period', header: '结算周期', className: 'font-mono text-xs', render: (row) => row.period },
  { id: 'currency', header: '币种', align: 'center', className: 'w-[64px] font-mono', render: (row) => row.currency },
  {
    id: 'total',
    header: '对账单总额',
    align: 'right',
    className: 'font-mono text-xs',
    // 总额照实转写不由行数派生：总额严格等于所含明细之和是写口的不变量，读侧重算一遍只会
    // 在两处各说一套。
    render: (row) => row.totalAmount,
  },
  {
    id: 'line-count',
    header: '费用行数',
    align: 'right',
    className: 'font-mono',
    render: (row) => String(row.lineCount),
  },
  {
    id: 'adjustment-count',
    header: '调整行数',
    align: 'right',
    className: 'font-mono',
    render: (row) => String(row.adjustmentCount),
  },
  {
    id: 'doc-state',
    header: '单据状态',
    // 二态由作废留痕渲染，不另造第二个布尔。作废不删行不改总额、替代单用新单号，所以
    // 「已作废」是这一行上的留痕而不是它的消失——依据与时刻随态一并呈现，指得出续办。
    render: (row) =>
      row.voidBasis ? (
        <div className="min-w-44 text-xs">
          <p>已作废</p>
          <p className="font-mono text-idpxyz-textMuted">
            {row.voidBasis}
            {row.voidedAt ? `（${formatInstant(row.voidedAt)}）` : ''}
          </p>
        </div>
      ) : (
        <span>已发布</span>
      ),
  },
  {
    id: 'published-at',
    header: '发布时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.publishedAt),
  },
];

function CustomerStatementsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<CustomerStatementListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSettlementStatements('customer-statement').then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const statements = answer?.kind === 'outcome' ? answer.body.statements : [];
  const needle = search.trim().toLowerCase();
  const visibleStatements = needle
    ? statements.filter((row) =>
        [row.statementNumber, row.account, row.period].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : statements;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CustomerStatementRecord>
      title="客户对账单"
      description={`${info.owner}——只上已发布对账单；异议不撤销原单，收付款也不改变单的内容`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索对账单号 / 结算账户 / 结算周期',
      }}
      filterSummary={
        answer?.kind === 'outcome' ? `当前返回 ${statements.length} 张对账单` : undefined
      }
      columns={customerStatementColumns}
      rows={visibleStatements}
      // 行键即对账单号：发布后固定，替代单用新单号。
      rowKey={(row) => row.statementNumber}
      viewState={catalogueViewState(answer, statements.length, retry, {
        module: info,
        endpoint: 'GET /settlement-statements?registry=customer-statement',
        emptyTitle: '当前租户尚无已发布的客户对账单',
        emptyDescription:
          '读取入口已配置，但对账单登记册为空；对账单由 UC-SA-003 按账期归集已确认费用形成，事务链在接入渠道墙后面，页面不会预置对账单。',
      })}
    />
  );
}

// —— 供应商账单主张签 ——

const supplierBillReceptionColumns: ListColumn<SupplierBillReceptionRecord>[] = [
  { id: 'claim', header: '主张标识', className: 'font-mono text-xs', render: (row) => row.claim },
  { id: 'claim-version', header: '主张版本', className: 'font-mono text-xs', render: (row) => row.claimVersion },
  { id: 'supplier', header: '供应商', className: 'font-mono text-xs', render: (row) => row.supplier },
  { id: 'legal-entity', header: '责任法人', className: 'font-mono text-xs', render: (row) => row.legalEntity },
  { id: 'period', header: '账期', className: 'font-mono text-xs', render: (row) => row.period },
  { id: 'currency', header: '币种', align: 'center', className: 'w-[64px] font-mono', render: (row) => row.currency },
  {
    id: 'line-count',
    header: '明细行数',
    align: 'right',
    className: 'font-mono',
    render: (row) => String(row.lineCount),
  },
  {
    id: 'match-count',
    header: '匹配行数',
    align: 'right',
    className: 'font-mono',
    // 与明细行数并列而不折成一个「匹配率」：未匹配、部分匹配与已匹配是三种结果，
    // 一个比率会把它们压成同一格。
    render: (row) => String(row.matchCount),
  },
  {
    id: 'audit-authority',
    header: '审核授权',
    align: 'center',
    // 照实转写而不折成「可审核」：授权未配置时审核停在未决，不默认放行也不虚构授权人；
    // 译成一个动作可用性就是在读面上替审核步骤做了那个判断。
    render: (row) =>
      row.auditAuthorityConfigured ? (
        <span>已配置</span>
      ) : (
        <span className="text-idpxyz-textMuted">未配置</span>
      ),
  },
  {
    id: 'recorded-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.recordedAt),
  },
];

function SupplierBillReceptionsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<SupplierBillReceptionListResponseBody> | null>(
    null,
  );

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
        [row.claim, row.supplier, row.legalEntity, row.period].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : receptions;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<SupplierBillReceptionRecord>
      title="供应商账单主张"
      description={`${info.owner}——主张是供应商侧的说法，不是审核应付；未审核、存在争议或被拒绝的金额不得混入审核应付`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索主张标识 / 供应商 / 责任法人 / 账期',
      }}
      filterSummary={
        answer?.kind === 'outcome' ? `当前返回 ${receptions.length} 份主张` : undefined
      }
      columns={supplierBillReceptionColumns}
      rows={visibleReceptions}
      // 行键循主张加版本：主张换版本另成一行，原版本保留。
      rowKey={(row) => `${row.claim}:${row.claimVersion}`}
      viewState={catalogueViewState(answer, receptions.length, retry, {
        module: info,
        endpoint: 'GET /settlement-statements?registry=supplier-bill-reception',
        emptyTitle: '当前租户尚无已登记的供应商账单主张',
        emptyDescription:
          '读取入口已配置，但供应商账单主张登记册为空；主张由 UC-SA-004 接收登记，事务链在接入渠道墙后面，页面不会预置主张。',
      })}
    />
  );
}

/**
 * 对账与核销。本页承载对账单视角的两册：客户对账单（UC-SA-003）与供应商账单主张
 * （UC-SA-004）。核销（收付款分配到未结项）已按 UC-SA-005 口径独立成页，见同目录
 * SettlementApplicationPage——收付款事实与核销关系同对账单分别管理，收付款不改变对账单内容，
 * 故不在本页另立区块。
 *
 * 本页是查阅面，不设发布、作废或异议裁定动作——那些各由自己的唯一创建用例形成。
 */
export function ReconciliationPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="customer" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="customer">客户对账单</TabsTrigger>
          <TabsTrigger value="supplier">供应商账单主张</TabsTrigger>
        </TabsList>
        <TabsContent value="customer" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomerStatementsTable />
        </TabsContent>
        <TabsContent value="supplier" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <SupplierBillReceptionsTable />
        </TabsContent>
      </Tabs>
    </div>
  );
}
