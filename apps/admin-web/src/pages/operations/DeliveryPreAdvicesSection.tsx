import { useEffect, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn, type TemplateViewState } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listDeliveryPreAdvices,
  readDeliveryPreAdvice,
  type ApiResult,
  type DeliveryPreAdviceFoundResponseBody,
  type DeliveryPreAdviceListResponseBody,
} from './delivery-scope-api';
import {
  deliveryScopeListPathOf,
  preAdviceFilterKeys,
  preAdviceRowOf,
  type DeliveryScopeListFilter,
  type PreAdviceReviewRow,
} from './delivery-pre-advice';
import { DeliveryScopeFilters, DeliveryScopeReadNote } from './DeliveryScopeFilters';

const info = moduleInfoById['node-operations-review'];
const listBase = '/node-operations-delivery-pre-advices';

function col(id: string, header: string, mono = false): ListColumn<PreAdviceReviewRow> {
  return {
    id,
    header,
    render: (row) => (mono ? <span className="font-mono text-[12px]">{row.values[id] ?? '—'}</span> : (row.values[id] ?? '—')),
  };
}

const columns: ListColumn<PreAdviceReviewRow>[] = [
  col('preAdviceId', '预报标识', true),
  col('customerAccount', '客户账户', true),
  col('node', '物流节点', true),
  col('expectedArrivalAt', '预计送达', true),
  col('numbers', '客户包裹件号'),
  col('deliveringParty', '交付方线索', true),
  col('withdrawal', '撤回'),
  col('formedAt', '登记时刻', true),
];

/**
 * 节点作业查阅页的「送站预报」区（票 ps-external-mark-relations/13；UC-NO-006）：按节点与预计送达日列客户经客户渠道交来的预报，点一行看
 * 号码集合。只读——预报由客户账户自己提交、撤回（首方客户渠道），运营在这里不登记、不改、不撤；一份预报对哪次送站算数，在收寄时由节点、
 * 时窗与号码逐条判（ADR-0212 决定二），这一区不预判。
 */
export function DeliveryPreAdvicesSection({ chips }: { chips: ReactNode }) {
  const [filter, setFilter] = useState<DeliveryScopeListFilter>({ node: '', day: '' });
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{ path: string; answer: ApiResult<DeliveryPreAdviceListResponseBody> } | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const path = deliveryScopeListPathOf(listBase, preAdviceFilterKeys, filter);

  useEffect(() => {
    if (path === null) return undefined;
    let cancelled = false;
    void listDeliveryPreAdvices(path).then((answer) => {
      if (!cancelled) setLoaded({ path, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [path, reloadKey]);

  const answer = path !== null && loaded?.path === path ? loaded.answer : null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? body.preAdvices.map(preAdviceRowOf) : [];
  const retry = () => setReloadKey((value) => value + 1);
  const narrowed = filter.node.trim() !== '' || filter.day !== '';
  const viewState: TemplateViewState =
    path === null
      ? { kind: 'error', title: '预计送达日认不出', description: '日期格不是一个日历上存在的日子；改正后才发请求，页面不发半截条件。' }
      : catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: `GET ${listBase}`,
          emptyTitle: '当前租户尚无送站预报',
          emptyDescription: '读取入口已配置，预报册为空——预报由客户账户经首方客户渠道交来；页面不含合成数据。',
          narrowed,
        });

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <ListPageTemplate<PreAdviceReviewRow>
        title={info.title}
        description={`${info.owner} · 送站预报是客户账户在送站之前交来的到货计划：送到哪个节点、预计何时送达、送来哪些客户包裹件号。本区只读；点一行看号码集合。`}
        filters={<DeliveryScopeFilters chips={chips} filter={filter} dayLabel="预计送达日" onChange={setFilter} />}
        filterSummary={body ? `送站预报 ${rows.length} 行` : undefined}
        emptyRowsNote={narrowed ? '当前筛选条件下没有预报' : undefined}
        columns={columns}
        rows={rows}
        rowKey={(row) => row.key}
        onRowClick={(row) => setSelected(row.preAdviceId)}
        viewState={viewState}
      />
      {selected ? <PreAdviceDetail key={selected} preAdviceId={selected} onClose={() => setSelected(null)} /> : null}
    </div>
  );
}

function PreAdviceDetail({ preAdviceId, onClose }: { preAdviceId: string; onClose: () => void }) {
  const [answer, setAnswer] = useState<ApiResult<DeliveryPreAdviceFoundResponseBody> | null>(null);
  useEffect(() => {
    let cancelled = false;
    void readDeliveryPreAdvice(preAdviceId).then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [preAdviceId]);

  return (
    <Card className="mx-4 mb-4">
      <CardHeader>
        <CardTitle>
          送站预报 <span className="font-mono">{preAdviceId}</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2 text-xs">
        {answer === null ? <p className="text-idpxyz-textMuted">读取中…</p> : null}
        {answer !== null && answer.kind !== 'outcome' ? <DeliveryScopeReadNote answer={answer} what="查阅送站预报" /> : null}
        {answer?.kind === 'outcome' ? (
          <dl className="grid grid-cols-[120px_1fr] gap-x-3 gap-y-1">
            <dt className="text-idpxyz-textMuted">客户账户</dt>
            <dd className="font-mono">{answer.body.preAdvice.customerAccount}</dd>
            <dt className="text-idpxyz-textMuted">物流节点</dt>
            <dd className="font-mono">{answer.body.preAdvice.node}</dd>
            <dt className="text-idpxyz-textMuted">预计送达</dt>
            <dd className="font-mono">{formatInstant(answer.body.preAdvice.expectedArrivalAt)}</dd>
            <dt className="text-idpxyz-textMuted">交付方线索</dt>
            <dd className="font-mono">{answer.body.preAdvice.deliveringParty ?? '（没给）——线索不参与范围判断'}</dd>
            <dt className="text-idpxyz-textMuted">撤回</dt>
            <dd>
              {answer.body.preAdvice.withdrawn
                ? `已撤回${answer.body.preAdvice.withdrawnAt ? ` · ${formatInstant(answer.body.preAdvice.withdrawnAt)}` : ''}——此后的收寄与重核不再据它成立范围，此前据它形成的关联不回滚`
                : '未撤回'}
            </dd>
            <dt className="text-idpxyz-textMuted">客户包裹件号</dt>
            <dd>
              <ul className="font-mono">
                {answer.body.preAdvice.customerParcelNumbers.map((number) => (
                  <li key={number}>{JSON.stringify(number)}</li>
                ))}
              </ul>
              <span className="text-idpxyz-textMuted">号码逐字保存、逐字比对（带引号示出，空白与不可见字符看得见）</span>
            </dd>
          </dl>
        ) : null}
        <div>
          <Button variant="outline" onClick={onClose}>
            收起
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
