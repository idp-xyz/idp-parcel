import { ListPageTemplate, useAddressKeyword, type ListColumn } from '../../templates';
import { StatusPill } from '../../components/StatusPill';
import { CodeRef } from '../../components/CodeRef';
import { labelFor } from '../../vocabulary/status';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { listLabelTransactions, type LabelTransactionRow } from './api';
import { ParcelNo } from './refs';

// 页面标题取导航登记的那一份;主责上下文(parcel-shipment)只在代码里出现,屏上不说系统的话。
const info = moduleInfoById['label-transactions'];

/**
 * 面单交易查阅面(已接线:GET /label-transactions)。词取 parcel-shipment CONTEXT.md
 * 「面单交易」「面单交易包裹结果」「面单交易定案」「面单继续尝试决定」原词;屏上按运营的叫法:交易结果、包裹结果、
 * 定案、还能不能再试。
 *
 * 行粒度是交易 × 包裹,摊开在服务端读侧完成(ADR-0084 决定七):一笔交易可覆盖多个包裹、
 * 一个包裹可因重试/替代/换单关联多笔交易,且「多包裹面单交易可以具有共同交易结果,也必须
 * 分别保存每个包裹的业务结果」——交易级与包裹级结果分列即这条硬句的表形,任何一级都推断
 * 不出另一级。
 *
 * 本页无关闭/重开动作入口:受控关闭与重开是授权业务角色形成的追加式决定(请求方、实际
 * 决定方、授权依据快照缺一不可),查阅面不取得决定权;页面也不设「作废 / 退款」入口——
 * 查询、重打、渠道作废、替换和渠道退款是不同业务动作,各走其责任入口。
 *
 * 「还能再试」一列由每件包裹的继续尝试决定登记册(受控关闭 / 重开决定)与当前有效终局按规则现算
 * (continuedAttemptBasis 说明派生依据),不是存下来的状态;派生依据的码不上屏。
 *
 * 读口已迁到 ADR-0144:检索词(地址上的 `?q=`,停稳后)下推,接模板的游标翻页,不在答回来的那一页上再筛。翻页按交易翻,
 * 翻页条上的总数数的是交易,一页交易摊开的行比它多,所以摘要把两样分开报。渠道面单号不在读口检索的列里,检索框的提示
 * 因此不写它;读口另有交易结果一维,本页没有对应的筛选条,不另加。
 */

/**
 * 包裹级结果的呈现。三态而不是两态:**结果未回**与**未受理**必须分开——把前者显示成
 * 后者,就是在页面上把「结果不确定」按失败处理,CONTEXT 明禁。
 */
function parcelResultCell(row: LabelTransactionRow) {
  if (!row.hasParcelResult) return <StatusPill label="结果未回" tone="warn" />;
  const followUps = row.followUpKinds.map((kind) => labelFor(kind)).join('、');
  return (
    <div className="flex flex-col gap-0.5">
      <span className="inline-flex items-center gap-2">
        <StatusPill label={row.parcelAccepted ? '已受理' : '未受理'} tone={row.parcelAccepted ? 'good' : 'bad'} />
        {row.parcelAccepted && row.parcelIdentifier ? <CodeRef value={row.parcelIdentifier} title="渠道面单号" /> : null}
      </span>
      {!row.parcelAccepted && row.parcelResultReason ? <span className="type-caption text-idpxyz-textMuted">{row.parcelResultReason}</span> : null}
      {/* 后续动作与原结果并列而不改写它:渠道作废与「当初有没有被受理」是两件事。 */}
      {followUps ? <span className="type-caption text-idpxyz-textMuted">之后:{followUps}</span> : null}
    </div>
  );
}

/**
 * 「还能再试」一格的呈现。判断值只有两格(CONTEXT 原词:开放 / 受控关闭),但「开放」有两种来源——
 * CONTEXT 生命周期那句「尚无生效的受控关闭**或**最近适用决定为重开」的两半——现场处置相反:
 * 前者没有任何关闭册可查,后者要去看那份重开依据的什么。不给判断加第三格(那是新造领域
 * 语言),来源用第二行小字交代。受控关闭不再细分:它可能来自生效关闭,也可能只是当前有效
 * 终局在场,两者都不允许边界后的新尝试。
 */
function continuedAttemptCell(row: LabelTransactionRow) {
  if (!row.continuedAttemptOpen) return <StatusPill label="不能再试" tone="muted" />;
  return (
    <div className="flex flex-col items-center gap-0.5">
      <StatusPill label="可以再试" tone="info" />
      <span className="type-caption text-idpxyz-textMuted">{row.continuedAttemptDecided ? '关过又重开' : '没有人关过'}</span>
    </div>
  );
}

const columns: ListColumn<LabelTransactionRow>[] = [
  {
    id: 'parcel-id',
    header: '包裹号',
    className: 'w-[200px]',
    render: (row) => <ParcelNo parcelId={row.parcelId} />,
  },
  {
    id: 'transaction-id',
    header: '面单交易',
    className: 'w-[200px]',
    render: (row) => (
      <div className="flex flex-col gap-0.5">
        <CodeRef value={row.transactionId} title="面单交易" />
        {row.priorTransactionId && (
          <span className="type-caption text-idpxyz-textMuted">
            {labelFor(row.priorLinkKind ?? '')}自 <span className="font-mono">{row.priorTransactionId}</span>
          </span>
        )}
      </div>
    ),
  },
  { id: 'channel-servicer', header: '渠道', render: (row) => row.channelServicer },
  { id: 'account-holder', header: '渠道账号属于', render: (row) => row.accountHolder },
  { id: 'counterparty', header: '结算对象', render: (row) => row.settlementCounterparty },
  { id: 'transaction-result', header: '交易结果', render: (row) => <StatusPill code={row.transactionResult} /> },
  { id: 'parcel-result', header: '包裹结果', render: (row) => parcelResultCell(row) },
  {
    id: 'settlement-status',
    header: '定案',
    align: 'center',
    className: 'w-[112px]',
    // 定案 ≠ 成功:定下的可能是失败,所以不按成功色画,只说结果不再变。
    render: (row) => (row.finalized ? <StatusPill label="已定案" tone="muted" /> : <StatusPill label="待确认" tone="warn" />),
  },
  { id: 'continue-attempt', header: '还能再试', align: 'center', className: 'w-[120px]', render: (row) => continuedAttemptCell(row) },
  { id: 'business-time', header: '时间', className: 'w-[170px]', render: (row) => <span className="type-body">{formatInstant(row.establishedAt)}</span> },
];

export function LabelTransactionsPage() {
  const [keyword, setKeyword] = useAddressKeyword();
  const conditions = { q: useSettledKeyword(keyword) };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listLabelTransactions);

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body?.rows ?? [];
  const transactionCount = new Set(rows.map((row) => row.transactionId)).size;

  return (
    <ListPageTemplate<LabelTransactionRow>
      title={info.title}
      description="每笔面单交易覆盖了哪些包裹、渠道答了什么、还能不能再试。定案只说明结果不再变,定下的也可能是失败。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜包裹号 / 面单交易 / 渠道账号' }}
      filterSummary={body ? `本页 ${transactionCount} 笔交易,摊开 ${rows.length} 行` : undefined}
      columns={columns}
      rows={rows}
      rowKey={(row) => `${row.transactionId}#${row.parcelId}`}
      emptyRowsNote="当前检索条件下没有匹配的面单交易"
      pagination={pagination}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /label-transactions',
        emptyTitle: '还没有面单交易',
        emptyDescription: '面单交易由渠道对接写入;渠道接上之前这里是空的。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}
