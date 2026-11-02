import { useEffect, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import type { ListColumn, TemplateViewState } from '../../templates';
import { RouteWorkbench } from '../network/route-workbench';
import { useCustomerNames } from '../shipment-request/use-customer-names';
import type { CustomerNameIndex } from '../shipment-request/shipment-request-list';
import { reviewStatusOf, workbenchColumnsOf } from './review-workbench';
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

// 客户一格显名称不显编码（名称从客户账户册转写，读不到显编码，同运单页）；编码仍在行值里，检索照样命中。
function columnsWith(names: CustomerNameIndex): ListColumn<PreAdviceReviewRow>[] {
  return columns.map((column) =>
    column.id === 'customerAccount'
      ? { ...column, render: (row) => names.get(row.values.customerAccount) ?? <span className="font-mono text-[12px]">{row.values.customerAccount}</span> }
      : column,
  );
}

const columns: ListColumn<PreAdviceReviewRow>[] = [
  col('preAdviceId', '预报号', true),
  col('customerAccount', '客户', true),
  col('node', '送到哪个网点', true),
  col('expectedArrivalAt', '预计送达', true),
  col('numbers', '客户的包裹号'),
  col('deliveringParty', '谁来送', true),
  col('withdrawal', '撤回了没有'),
  col('formedAt', '收到预报的时间', true),
];

/**
 * 节点作业查阅页的「送站预报」区（票 ps-external-mark-relations/13；UC-NO-006）：按节点与预计送达日列客户经客户渠道交来的预报，点一行看
 * 号码集合。只读——预报由客户账户自己提交、撤回（首方客户渠道），运营在这里不登记、不改、不撤；一份预报对哪次送站算数，在收寄时由节点、
 * 时窗与号码逐条判（ADR-0212 决定二），这一区不预判。
 */
export function DeliveryPreAdvicesSection({ sections }: { sections: ReactNode }) {
  const [filter, setFilter] = useState<DeliveryScopeListFilter>({ node: '', day: '' });
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{ path: string; answer: ApiResult<DeliveryPreAdviceListResponseBody> } | null>(null);
  const names = useCustomerNames();
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
      ? { kind: 'error', title: '预计送达日期写得不对', description: '这一天在日历上不存在；改正日期后再查。' }
      : catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: `GET ${listBase}`,
          emptyTitle: '还没有客户送货预报',
          emptyDescription: '客户在送货到网点之前提交预报（送到哪个网点、几点到、送哪些包裹），提交后会出现在这里。',
          narrowed,
        });

  return (
    <RouteWorkbench<PreAdviceReviewRow>
      title={info.title}
      subtitle="客户送货到网点之前交来的到货计划：送到哪个网点、预计几点到、送哪些包裹。这里只查看；点一行看包裹号清单。"
      sections={sections}
      extraFilters={<DeliveryScopeFilters filter={filter} dayLabel="预计送达日" onChange={setFilter} />}
      answered={body !== null}
      noun="份"
      rows={rows}
      rowKey={(row) => row.key}
      statusOf={(row) => reviewStatusOf(row, { labelKey: 'withdrawal' })}
      searchCells={(row) => Object.values(row.values)}
      searchPlaceholder="在结果里搜预报号 / 客户 / 网点"
      columns={workbenchColumnsOf(columnsWith(names), { labelKey: 'withdrawal' })}
      renderDetail={(row, close) => (
        <div className="h-full overflow-y-auto bg-idpxyz-bg p-4">
          <PreAdviceDetail key={row.preAdviceId} preAdviceId={row.preAdviceId} onClose={close} />
        </div>
      )}
      onRefresh={retry}
      viewState={viewState}
    />
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
    <Card>
      <CardHeader>
        <CardTitle>
          客户送货预报 <span className="font-mono">{preAdviceId}</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2 text-xs">
        {answer === null ? <p className="text-idpxyz-textMuted">读取中…</p> : null}
        {answer !== null && answer.kind !== 'outcome' ? <DeliveryScopeReadNote answer={answer} what="查看送货预报" /> : null}
        {answer?.kind === 'outcome' ? (
          <dl className="grid grid-cols-[120px_1fr] gap-x-3 gap-y-1">
            <dt className="text-idpxyz-textMuted">客户</dt>
            <dd className="font-mono">{answer.body.preAdvice.customerAccount}</dd>
            <dt className="text-idpxyz-textMuted">送到哪个网点</dt>
            <dd className="font-mono">{answer.body.preAdvice.node}</dd>
            <dt className="text-idpxyz-textMuted">预计送达</dt>
            <dd className="font-mono">{formatInstant(answer.body.preAdvice.expectedArrivalAt)}</dd>
            <dt className="text-idpxyz-textMuted">谁来送</dt>
            <dd className="font-mono">{answer.body.preAdvice.deliveringParty ?? '（没写）——只是线索，不影响送货范围的判断'}</dd>
            <dt className="text-idpxyz-textMuted">撤回</dt>
            <dd>
              {answer.body.preAdvice.withdrawn
                ? `已撤回${answer.body.preAdvice.withdrawnAt ? ` · ${formatInstant(answer.body.preAdvice.withdrawnAt)}` : ''}——之后收货不再按它认送货范围，之前按它认下的不撤销`
                : '未撤回'}
            </dd>
            <dt className="text-idpxyz-textMuted">客户的包裹号</dt>
            <dd>
              <ul className="font-mono">
                {answer.body.preAdvice.customerParcelNumbers.map((number) => (
                  <li key={number}>{JSON.stringify(number)}</li>
                ))}
              </ul>
              <span className="text-idpxyz-textMuted">号码逐字保存、逐字比对；带引号显示，好看出多余的空格</span>
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
