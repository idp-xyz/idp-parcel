import { useEffect, useState, type ReactNode } from 'react';
import type { ListColumn, TemplateViewState } from '../../templates';
import { RecordDetailPanel, RouteWorkbench, techRows } from '../network/route-workbench';
import { codeStatus } from '../visibility/exception-list';
import { useCustomerNames } from '../shipment-request/use-customer-names';
import type { CustomerNameIndex } from '../shipment-request/shipment-request-list';
import { withPlaceNames, workbenchColumnsOf } from './review-workbench';
import { usePlaceNames } from '../network/use-place-names';
import { placeLabel } from '../network/place-names';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { catalogueConditionsNarrow, type CatalogueConditions } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword, type CatalogueCursor } from '../use-catalogue-cursor';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { PageOnlyNote, PageOnlySearch } from '../visibility/page-only';
import { documentTypeWords } from '../../vocabulary/document-numbering';
import {
  listDeliveryPreAdvices,
  readDeliveryPreAdvice,
  type ApiResult,
  type DeliveryPreAdviceFoundResponseBody,
  type DeliveryPreAdviceListResponseBody,
} from './delivery-scope-api';
import {
  deliveryScopeConditions,
  deliveryScopeFilterNarrow,
  documentNumberDetailOf,
  preAdviceFilterKeys,
  preAdviceRowOf,
  type DeliveryScopeListFilter,
  type PreAdviceReviewRow,
} from './delivery-pre-advice';
import { DeliveryScopeFilters, DeliveryScopeReadNote, type DeliveryScopeControls } from './DeliveryScopeFilters';
import { allStatuses, pageStatusChips, rowsOnPage } from './list-conditions';

const info = moduleInfoById['node-operations-review'];
// 号名取单据编号词典（设置 → 单据编号同一份），不在本页另起叫法。
const numberName = documentTypeWords.DELIVERY_PRE_ADVICE.numberName;
const listBase = '/node-operations-delivery-pre-advices';

function col(id: string, header: string, mono = false): ListColumn<PreAdviceReviewRow> {
  return {
    id,
    header,
    render: (row) => (mono ? <span className="font-mono type-body">{row.values[id] ?? '—'}</span> : (row.values[id] ?? '—')),
  };
}

// 客户一格显名称不显编码（名称从客户账户册转写，读不到显编码，同运单页）；编码仍在行值里，检索照样命中。
function columnsWith(names: CustomerNameIndex): ListColumn<PreAdviceReviewRow>[] {
  return columns.map((column) =>
    column.id === 'customerAccount'
      ? { ...column, render: (row) => names.get(row.values.customerAccount) ?? <span className="font-mono type-body">{row.values.customerAccount}</span> }
      : column,
  );
}

// 主列是预报的单据号（ADR-0355 决定十一）；上线前开立、开立时单据编号时区未配置的预报没有单据号，这一格照旧显内部标识，原因在技术详情。
const columns: ListColumn<PreAdviceReviewRow>[] = [
  col('documentNumber', numberName, true),
  col('customerAccount', '客户', true),
  col('node', '送到哪个网点'),
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
 *
 * 列表接读口分页（ADR-0144，票 catalogue-read-pagination-rollout/14）：检索框下推成 `q`；节点、日子与单据号是读口的选择器，照旧交服务端。
 */
export function DeliveryPreAdvicesSection({ sections }: { sections: ReactNode }) {
  const [filter, setFilter] = useState<DeliveryScopeListFilter>({ node: '', day: '', documentNumber: '' });
  const [keyword, setKeyword] = useState('');
  const conditions = deliveryScopeConditions(preAdviceFilterKeys, filter, useSettledKeyword(keyword));
  const controls: DeliveryScopeControls = { sections, filter, onFilter: setFilter, keyword, onKeyword: setKeyword };
  // 日子认不出时不发请求（deliveryScopeConditions 答 null）：取数的那一件只在条件成立时挂上。
  return conditions === null ? (
    <PreAdviceWorkbench controls={controls} cursor={null} />
  ) : (
    <PreAdviceCursor controls={controls} conditions={conditions} />
  );
}

function PreAdviceCursor({ controls, conditions }: { controls: DeliveryScopeControls; conditions: CatalogueConditions }) {
  return <PreAdviceWorkbench controls={controls} conditions={conditions} cursor={useCatalogueCursor(conditions, listDeliveryPreAdvices)} />;
}

// 撤回与否是读口派生的状态（withdrawn 恒在场），不是筛选维：胶囊只筛本页。
const withdrawnStateOf = (row: PreAdviceReviewRow) => row.values.withdrawnState;

function PreAdviceWorkbench({
  controls,
  conditions,
  cursor,
}: {
  controls: DeliveryScopeControls;
  conditions?: CatalogueConditions;
  cursor: CatalogueCursor<DeliveryPreAdviceListResponseBody> | null;
}) {
  const { sections, filter, onFilter, keyword, onKeyword } = controls;
  const [status, setStatus] = useState(allStatuses);
  const [refine, setRefine] = useState('');
  const names = useCustomerNames();
  const places = usePlaceNames();
  const answer = cursor?.answer ?? null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const pageRows = body ? withPlaceNames(body.preAdvices.map(preAdviceRowOf), ['node'], places) : [];
  const rows = rowsOnPage(pageRows, refine, status, withdrawnStateOf);
  const viewState: TemplateViewState =
    cursor === null
      ? { kind: 'error', title: '预计送达日期写得不对', description: '这一天在日历上不存在；改正日期后再查。' }
      : catalogueViewState(answer, pageRows.length, cursor.retry, {
          module: info,
          endpoint: `GET ${listBase}`,
          emptyTitle: '还没有客户送货预报',
          emptyDescription: '客户在送货到网点之前提交预报（送到哪个网点、几点到、送哪些包裹），提交后会出现在这里。',
          narrowed:
            deliveryScopeFilterNarrow(filter) ||
            (conditions !== undefined && catalogueConditionsNarrow(conditions)) ||
            pastFirstPage(cursor.pagination),
        });

  return (
    <RouteWorkbench<PreAdviceReviewRow>
      title={info.title}
      subtitle="客户送货到网点之前交来的到货计划：送到哪个网点、预计几点到、送哪些包裹。这里只查看；点一行看包裹号清单。"
      sections={sections}
      extraFilters={
        <>
          <DeliveryScopeFilters filter={filter} dayLabel="预计送达日" numberLabel={numberName} onChange={onFilter} />
          {body ? <PageOnlyNote>撤回与否只筛本页</PageOnlyNote> : null}
          <PageOnlySearch value={refine} onChange={setRefine} subject="网点名称、谁来送等其余各格" />
        </>
      }
      answered={body !== null}
      noun="份"
      rows={rows}
      rowKey={(row) => row.key}
      statusChips={body ? pageStatusChips(pageRows.map(withdrawnStateOf), status, setStatus) : undefined}
      onClearFilters={() => {
        setStatus(allStatuses);
        setRefine('');
        onFilter({ node: '', day: '', documentNumber: '' });
      }}
      searchCells={() => []}
      search={{ value: keyword, onChange: onKeyword, pushedDown: true }}
      searchPlaceholder={`搜${numberName}、预报标识、客户编码、网点编码`}
      columns={workbenchColumnsOf(columnsWith(names), { labelKey: 'withdrawal' })}
      renderDetail={(row, close) => <PreAdviceDetail key={row.preAdviceId} preAdviceId={row.preAdviceId} onClose={close} />}
      onRefresh={cursor?.retry}
      toolbarRight={cursor?.pagination ? <CursorPager pagination={cursor.pagination} /> : undefined}
      viewState={viewState}
    />
  );
}

/**
 * 一份预报的右栏，形状同本区其余几页：客户、送到哪个网点、预计几点到、谁来送、撤回了没有是事实格；包裹号清单在正文，逐字显示；
 * 单据号（没有时按词典注明为什么没有）、预报标识、客户与网点编码、收到与撤回的时刻进技术详情。标题是单据号，没有单据号的照旧用预报标识。
 */
function PreAdviceDetail({ preAdviceId, onClose }: { preAdviceId: string; onClose: () => void }) {
  const [answer, setAnswer] = useState<ApiResult<DeliveryPreAdviceFoundResponseBody> | null>(null);
  const names = useCustomerNames();
  const places = usePlaceNames();
  useEffect(() => {
    let cancelled = false;
    void readDeliveryPreAdvice(preAdviceId).then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [preAdviceId]);

  const advice = answer?.kind === 'outcome' ? answer.body.preAdvice : null;
  return (
    <RecordDetailPanel
      title={advice?.documentNumber ?? preAdviceId}
      status={advice ? (advice.withdrawn ? codeStatus('WITHDRAWN') : { label: '未撤回', tone: 'info' }) : undefined}
      subtitle="客户送货预报"
      facts={
        advice
          ? [
              { label: '客户', value: names.get(advice.customerAccount) ?? <span className="font-mono">{advice.customerAccount}</span> },
              { label: '送到哪个网点', value: placeLabel(places, advice.node) },
              { label: '预计送达', value: formatInstant(advice.expectedArrivalAt) },
              { label: '谁来送', value: advice.deliveringParty ? <span className="font-mono">{advice.deliveringParty}</span> : '没写' },
            ]
          : []
      }
      technical={techRows([
        { label: numberName, value: advice ? documentNumberDetailOf(advice) : undefined },
        { label: '预报标识', value: preAdviceId },
        { label: '客户编码', value: advice?.customerAccount },
        { label: '网点编码', value: advice?.node },
        { label: '收到预报', value: advice ? formatInstant(advice.formedAt) : undefined },
        { label: '撤回时间', value: advice?.withdrawnAt ? formatInstant(advice.withdrawnAt) : undefined },
      ])}
      onClose={onClose}
    >
      {answer === null ? <p>读取中…</p> : null}
      {answer !== null && answer.kind !== 'outcome' ? <DeliveryScopeReadNote answer={answer} what="查看送货预报" /> : null}
      {advice ? (
        <>
          {advice.withdrawn ? <p>客户已撤回这份预报：之后收货不再按它认送货范围，之前按它认下的不撤销。</p> : null}
          <p>「谁来送」只是线索，不影响送货范围的判断。</p>
          <p className="font-medium text-idpxyz-text">客户的包裹号（{advice.customerParcelNumbers.length}）</p>
          <ul className="font-mono text-idpxyz-text">
            {advice.customerParcelNumbers.map((number) => (
              <li key={number}>{JSON.stringify(number)}</li>
            ))}
          </ul>
          <p>号码逐字保存、逐字比对；带引号显示，好看出多余的空格。</p>
        </>
      ) : null}
    </RecordDetailPanel>
  );
}
