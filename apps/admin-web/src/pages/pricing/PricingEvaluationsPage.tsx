import { useState } from 'react';
import { ListPageTemplate, useAddressKeyword, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { technicalRows } from '../settlement/ops-ui';
import { CountChips } from './CountChips';
import { listPricingEvaluations, type PricingEvaluationRecord } from './api';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['pricing-evaluation'];

// 计费记录（领域里叫「价格评价」）：每算一次价就留一条，争议复核时按它回放。读口只交编号、结果、两条摘要、规范化版本与
// 登记时间——金额与算法细节住在评价快照里，列面不透出，所以本页不显示金额，也不用 0 顶替（CONTEXT 硬句「不得以金额为零的
// 已完成评价表达不可计价」）。两条摘要与规范化版本是争议复核对账用的比对串，运营不每天看，进「技术详情」，展开后全文
// 可复制（截半就不能比对）。
//
// 结果五格照词典的 pricing-evaluation 分册译：「未形成」是该重试的故障，「不可计价」是终局业务判断，两词分开、色也分开。
//
// 读口已迁到 ADR-0144，按游标翻页（页底翻页条）。检索词（地址上的 `?q=`，停稳后）与结果胶囊下推成 q 与 status 筛选维，不在
// 答回来的那一页上再筛；q 覆盖计费编号与两条摘要（比对场景手里拿的就是这三样）。胶囊不带计数：一页里数出来的不是各结果的总数，
// 当前条件下共几条由翻页条说。

const STATUS_SCOPE = 'pricing-evaluation';

// 结果胶囊的固定顺序：先要人看的，再已完成。五格就是读口 status 一维的词表；集外取值不进胶囊，但行照常显示、胶囊格如实露码。
const statusOrder = ['PENDING', 'CONFLICT', 'FAILED', 'UNRATABLE', 'COMPLETED'] as const;

function evaluationTechnicalRows(row: PricingEvaluationRecord) {
  return technicalRows([
    { label: '计费编号', value: row.evaluationId },
    { label: '结果码', value: row.status },
    { label: '计算内容摘要', value: row.semanticDigest },
    { label: '价格表内容摘要', value: row.planContentDigest },
    { label: '规范化版本', value: row.canonicalization },
  ]);
}

const columns: ListColumn<PricingEvaluationRecord>[] = [
  {
    id: 'evaluation',
    header: '计费编号',
    className: 'font-mono type-body leading-4 text-idpxyz-textBright',
    render: (row) => row.evaluationId,
  },
  {
    id: 'status',
    header: '结果',
    render: (row) => <StatusPill code={row.status} scope={STATUS_SCOPE} />,
  },
  {
    id: 'recorded-at',
    header: '计费时间',
    className: 'whitespace-nowrap font-mono type-body leading-4',
    render: (row) => formatInstant(row.recordedAt),
  },
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={evaluationTechnicalRows(row)} className="max-w-md open:min-w-[22rem]" />,
  },
];

/**
 * 计费记录：系统每次按价格表算价留下的记录，查争议、对账单时按编号找到那一次。
 * 页面没有「重算 / 修改」：算过的记录不改，要重算是发起一次新的计费。
 */
export function PricingEvaluationsPage() {
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useState<string | null>(null);
  const conditions = { q: useSettledKeyword(search), filters: { status: status === null ? [] : [status] } };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listPricingEvaluations);
  const evaluations = answer?.kind === 'outcome' ? answer.body.evaluations : [];

  return (
    <ListPageTemplate<PricingEvaluationRecord>
      title={info.title}
      description="系统每次按价格表算价都留一条记录；客户对费用有疑问、对账单对不上时，按计费编号来这里找到那一次"
      search={{ value: search, onChange: setSearch, placeholder: '搜计费编号或摘要' }}
      filters={
        <CountChips
          label="结果"
          chips={statusOrder.map((code) => ({ id: code, label: labelFor(code, STATUS_SCOPE) }))}
          value={status}
          onChange={setStatus}
        />
      }
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 条」会与状态区「这不是册为空」直接矛盾。
        answer?.kind === 'outcome' ? `本页 ${evaluations.length} 条` : undefined
      }
      columns={columns}
      rows={evaluations}
      rowKey={(row) => row.evaluationId}
      emptyRowsNote="当前条件下没有计费记录"
      pagination={pagination}
      viewState={catalogueViewState(answer, evaluations.length, retry, {
        module: info,
        endpoint: 'GET /pricing-evaluations',
        emptyTitle: '还没有计费记录',
        emptyDescription: '运单受理、收寄后系统按价格表算价，每算一次这里就多一条；页面不会预置记录。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}
