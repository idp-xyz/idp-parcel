import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { objectNouns } from '../../vocabulary/objects';
import { manualEntryWord } from '../../vocabulary/provenance';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import { technicalRows } from '../settlement/ops-ui';
import { exceptionViewState } from './exception-view-state';
import { etaPredictionRowOf, etaPredictionsEndpoint, listETAPredictions, type ETAPredictionRow } from './eta-predictions';

const info = moduleInfoById['tracking-projection'];

// 预计到达签：各预测线当前登记的那一版，新近预测的在前。这一签是运营查阅面，回答「现在登记的预计到达是什么、谁录的、凭什么」；
// 客户此刻看不看得到它由可见性规则判，不在这里显示，也不由这里的字段推。

const mono = 'whitespace-nowrap font-mono type-body leading-4';

const columns: ListColumn<ETAPredictionRow>[] = [
  { id: 'parcel', header: objectNouns.parcelNumber, className: mono, render: (row) => row.parcel },
  { id: 'milestone', header: '到哪一步', className: mono, render: (row) => row.milestone },
  { id: 'range', header: '预计到达（范围）', className: mono, render: (row) => row.range },
  { id: 'basis', header: '口径', className: 'whitespace-nowrap', render: (row) => row.basis },
  { id: 'entry', header: '来源', className: 'whitespace-nowrap', render: (row) => row.entryWord },
  { id: 'predicted', header: '预测时间', className: mono, render: (row) => row.predictedAt },
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => (
      <TechnicalDetails
        rows={technicalRows([
          { label: '录入来源', value: row.entry },
          { label: '录入时间', value: row.enteredAt },
          { label: '可信度', value: row.confidence },
          { label: '预测版本', value: row.version },
          { label: '上一版', value: row.priorVersion },
          { label: '模型', value: row.model },
          { label: '输入', value: row.inputs },
        ])}
        className="max-w-md open:min-w-[18rem]"
      />
    ),
  },
];

export function ETAPredictionsTable() {
  const [search, setSearch] = useState('');
  const conditions = { q: useSettledKeyword(search) };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listETAPredictions);
  const rows = answer?.kind === 'outcome' ? answer.body.predictions.map(etaPredictionRowOf) : [];
  const manual = rows.filter((row) => row.entryWord === manualEntryWord).length;
  return (
    <ListPageTemplate<ETAPredictionRow>
      title="预计到达"
      description="每个包裹到每一步、按口径各一行的预计到达时间，显示当前登记的那一版：是承运商告知的还是我们自己估的，是人工录入的还是接口送来的。这里是内部登记，客户能否看到由预计到达的展示规则另行判断，不要直接照此答复客户。"
      search={{ value: search, onChange: setSearch, placeholder: '搜包裹号、里程碑、预测版本…' }}
      filterSummary={answer?.kind === 'outcome' ? `本页 ${rows.length} 条 · 其中人工录入 ${manual} 条` : undefined}
      columns={columns}
      rows={rows}
      rowKey={(row) => row.key}
      pagination={pagination}
      viewState={exceptionViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET ${etaPredictionsEndpoint}`,
        subject: '预计到达',
        emptyTitle: '还没有预计到达',
        emptyDescription: '预计到达由预测来源送入，或在「录入预计到达」签里人工录入；还没有登记不等于不会到。',
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
      })}
    />
  );
}
