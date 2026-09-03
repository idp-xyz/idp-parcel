import { useEffect, useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listSettlementFundsApplications,
  type ExternalFundsFactListResponseBody,
  type ExternalFundsFactRecord,
} from './api';
import { fundsFactKindLabels, labelOf } from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['settlement-application'];

// 本页接 GET /settlement-funds-applications，不设 registry 分派——资金事实册只有一本，
// 封闭集为一时参数只会造出一个恒定值。读面类型取自 pages/settlement/api（理由同
// ReconciliationPage 的文件头：目录是导航分组，不是上下文边界）。
//
// 旧骨架各栏随对栏裁定处置：
//   方向（收款／付款）——**换栏为事实种类**。册上是封闭三格，三格里只有一格是「收到了钱」；
//     折成收／付两向会把「付款失败」与「资金退回」压进同一格。
//   对方、责任法人、结算账户——**全撤**。三者在册上都没有列（册级缺席）。骨架自注说
//     「身份由外部事实保存，本页不推断」——册上确实没保存，那就不该留栏请人推断。
//   分配状态（未分配／部分核销／已核销）——**撤栏**。三态词会把「从未核销」与「核销过又
//     撤销了」压成同一格，而这两态的续办相反：前者要人去分配，后者要人去看当初为什么撤。
//   已撤销金额——**加栏**。三个金额各算各的和摆开，正是为了让上面那两态分得开；撤销不删
//     历史，那一截金额必须仍看得见。
//
// 映射与核销两组挂载物不进列表，按详情面处置（同下方页面自注）。

const columns: ListColumn<ExternalFundsFactRecord>[] = [
  { id: 'fact', header: '资金事实标识', className: 'font-mono text-xs', render: (row) => row.fact },
  { id: 'source', header: '来源系统', className: 'font-mono text-xs', render: (row) => row.source },
  {
    id: 'kind',
    header: '事实种类',
    align: 'center',
    // 封闭三格照原样呈现。通知不在其中——运营方收到付款通知但财务未确认时保持待确认，
    // 词表跟着没有那一格。
    render: (row) => labelOf(fundsFactKindLabels, row.kind),
  },
  {
    id: 'amount',
    header: '原币金额',
    align: 'right',
    className: 'font-mono text-xs',
    render: (row) => `${row.amount} ${row.currency}`,
  },
  {
    id: 'applied',
    header: '已分配金额',
    align: 'right',
    className: 'font-mono text-xs',
    // 只加未撤销的核销。
    render: (row) => row.appliedAmount,
  },
  {
    id: 'reversed',
    header: '已撤销金额',
    align: 'right',
    className: 'font-mono text-xs',
    // 只加已撤销的。与上一栏是两笔各算各的和，不是一个净额。
    render: (row) => row.reversedAmount,
  },
  {
    id: 'unapplied',
    header: '未分配金额',
    align: 'right',
    className: 'font-mono text-xs',
    // 未分配是第一类结果不是尾差，不得被默认费用吞并。
    render: (row) => row.unappliedAmount,
  },
  {
    id: 'application-count',
    header: '核销笔数',
    align: 'right',
    className: 'font-mono',
    // 三个金额之外再给笔数，判据同代收分户账页的记账笔数：金额全为零时，「从未核销过」与
    // 「核销过、金额相抵为零」长着同一张脸，笔数是唯一分得开的照实转写。
    render: (row) => String(row.applicationCount),
  },
  { id: 'version', header: '版本', className: 'font-mono text-xs', render: (row) => row.version },
  {
    id: 'corrects',
    header: '更正回指',
    className: 'font-mono text-xs',
    // 与更正时刻成对缺席表示这是首采。更正形成新有效版本，原采用保留。
    render: (row) =>
      row.corrects ? (
        <div className="min-w-40 text-xs">
          <p>{row.corrects}</p>
          {row.correctedAt && (
            <p className="text-idpxyz-textMuted">{formatInstant(row.correctedAt)}</p>
          )}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">非更正</span>
      ),
  },
  {
    id: 'occurred-at',
    header: '业务时间',
    className: 'min-w-44 font-mono text-xs',
    // 外部事实的业务发生时刻，不是系统接收或写入时间。
    render: (row) => formatInstant(row.occurredAt),
  },
  {
    id: 'actions',
    header: '操作',
    align: 'center',
    // 动作只有「人工分配」：自动核销只在匹配结果唯一时由 UC-SA-005 编排形成，不提供任何
    // 「自动分配」入口。本票只建读面，接线不取得创建权，故保持 disabled——核销撤销随详情
    // 呈现另立，不塞进列表。
    render: () => (
      <Button size="sm" disabled>
        人工分配
      </Button>
    ),
  },
];

/**
 * 收付款核销（UC-SA-005 映射外部收付款并形成运营核销）。
 *
 * 独立成页而不是对账页内区块：核销的行对象是已采用的外部资金事实及其分配关系，未结项跨
 * 客户运营应收、供应商审核应付、赔付／退款义务、供应商费用贷项、追偿应收与代垫回收，
 * 不从属于对账单；CONTEXT「结算账户、对账与争议」明确收付款事实与核销关系同对账单分别
 * 管理，收付款不改变对账单内容。
 *
 * 真实收付映射与核销分配片段（目标金额身份、借贷方向、带方向金额、匹配／抵销依据）与核销
 * 撤销届时按 DetailPageTemplate 另立详情，不在本列表展开——映射不是核销，事实接收、金额
 * 责任确认、真实到账与核销是不同结果。
 */
export function SettlementApplicationPage() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<ExternalFundsFactListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSettlementFundsApplications().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const facts = answer?.kind === 'outcome' ? answer.body.facts : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。
  const visibleFacts = needle
    ? facts.filter((row) =>
        [row.fact, row.source, row.currency].some((value) => value.toLowerCase().includes(needle)),
      )
    : facts;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<ExternalFundsFactRecord>
      title={info.title}
      description={`${info.owner}——外部系统拥有真实收付事实；对方、责任法人与结算账户不在本册，页面不推断`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索资金事实标识 / 来源系统 / 币种',
      }}
      filterSummary={
        answer?.kind === 'outcome' ? `当前返回 ${facts.length} 笔资金事实` : undefined
      }
      columns={columns}
      rows={visibleFacts}
      // 行键循身份加版本：相同身份与版本只采用一次，更正形成新有效版本。
      rowKey={(row) => `${row.fact}:${row.version}`}
      viewState={catalogueViewState(answer, facts.length, retry, {
        module: info,
        endpoint: 'GET /settlement-funds-applications',
        emptyTitle: '当前租户尚无已采用的外部资金事实',
        emptyDescription:
          '读取入口已配置，但资金事实登记册为空；真实收付款由外部财务或支付系统拥有，经 UC-SA-005 采用后才入册，页面不会预置资金事实。',
      })}
    />
  );
}
