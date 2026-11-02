import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { technicalRows } from '../settlement/ops-ui';
import { CountChips } from './CountChips';
import {
  listPricingEvaluations,
  type PricingEvaluationListResponseBody,
  type PricingEvaluationRecord,
} from './api';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['pricing-evaluation'];

// 计费记录（领域里叫「价格评价」）：每算一次价就留一条，争议复核时按它回放。读口只交编号、结果、两条摘要、规范化版本与
// 登记时间——金额与算法细节住在评价快照里，列面不透出，所以本页不显示金额，也不用 0 顶替（CONTEXT 硬句「不得以金额为零的
// 已完成评价表达不可计价」）。两条摘要与规范化版本是争议复核对账用的比对串，运营不每天看，进「技术详情」，展开后全文
// 可复制（截半就不能比对）。
//
// 结果五格照词典的 pricing-evaluation 分册译：「未形成」是该重试的故障，「不可计价」是终局业务判断，两词分开、色也分开。

const STATUS_SCOPE = 'pricing-evaluation';

// 结果胶囊的固定顺序：先要人看的，再已完成。集外取值不进胶囊，但行照常显示、胶囊格如实露码。
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
    className: 'font-mono text-xs text-idpxyz-textBright',
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
    className: 'whitespace-nowrap font-mono text-xs',
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
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<PricingEvaluationListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listPricingEvaluations().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const evaluations = answer?.kind === 'outcome' ? answer.body.evaluations : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的行上做，不下推成查询参数——那要改端点契约。比对场景手里拿的就是编号或两条摘要，三样都能搜。
  const visibleEvaluations = evaluations.filter(
    (row) =>
      (status === null || row.status === status) &&
      (needle === '' ||
        [row.evaluationId, row.semanticDigest, row.planContentDigest].some((value) => value.toLowerCase().includes(needle))),
  );
  const retry = () => setReloadKey((value) => value + 1);
  const countOf = (code: string) => evaluations.filter((row) => row.status === code).length;

  return (
    <ListPageTemplate<PricingEvaluationRecord>
      title={info.title}
      description="系统每次按价格表算价都留一条记录；客户对费用有疑问、对账单对不上时，按计费编号来这里找到那一次"
      search={{ value: search, onChange: setSearch, placeholder: '搜计费编号' }}
      filters={
        answer?.kind === 'outcome' && evaluations.length > 0 ? (
          <CountChips
            label="结果"
            total={evaluations.length}
            // 只摆有行的结果，加上当前选中的那一格（选中后筛空也要能点回去）。
            chips={statusOrder
              .filter((code) => code === status || countOf(code) > 0)
              .map((code) => ({ id: code, label: labelFor(code, STATUS_SCOPE), count: countOf(code) }))}
            value={status}
            onChange={setStatus}
          />
        ) : undefined
      }
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 条」会与状态区「这不是册为空」直接矛盾。
        answer?.kind === 'outcome' ? `${visibleEvaluations.length} / ${evaluations.length} 条` : undefined
      }
      columns={columns}
      rows={visibleEvaluations}
      rowKey={(row) => row.evaluationId}
      emptyRowsNote="没有匹配的计费记录"
      viewState={catalogueViewState(answer, evaluations.length, retry, {
        module: info,
        endpoint: 'GET /pricing-evaluations',
        emptyTitle: '还没有计费记录',
        emptyDescription: '运单受理、收寄后系统按价格表算价，每算一次这里就多一条；页面不会预置记录。',
      })}
    />
  );
}
