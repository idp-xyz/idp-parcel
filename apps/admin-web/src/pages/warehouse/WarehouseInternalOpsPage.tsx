import { useState } from 'react';
import { Button, Input, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorContent } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { catalogueConditionsNarrow, type CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import {
  listInternalJobs,
  queryOwnerStock,
  type ApiResult,
  type InternalJobRecord,
  type StockReadBody,
  type StockViewKind,
} from './api';
import { internalOpsActionSpecs } from './actions';
import { WarehouseActionPanel, useWarehouseActionTargets } from './ActionPanel';
import { problemNote } from './presentation';

const info = moduleInfoById['warehouse-internal-ops'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

type JobKindFilter = 'ALL' | 'INTERNAL_MOVE' | 'FORM_CONVERSION' | 'CONDITION_CONVERSION' | 'SHORTAGE';

const jobKindLabels: Record<string, string> = {
  INTERNAL_MOVE: '库内移动',
  FORM_CONVERSION: '形态转换',
  CONDITION_CONVERSION: '状态转换',
  SHORTAGE: '短少观察',
};

const stockViewLabels: Record<StockViewKind, string> = {
  cells: '库存格',
  cartons: '整箱',
  summary: '汇总',
  holds: '占用',
  movements: '流水',
};

const jobColumns: ListColumn<InternalJobRecord>[] = [
  { id: 'fact', header: '事实标识', className: mono, render: (row) => row.factId },
  { id: 'kind', header: '种类', render: (row) => jobKindLabels[row.kind] ?? row.kind },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouse },
  { id: 'lines', header: '行数', className: mono, render: (row) => String(row.lines.length) },
  { id: 'occurred', header: '发生时刻', className: mono, render: (row) => formatInstant(row.occurredAt) },
];

function jobInspector(row: InternalJobRecord): InspectorContent {
  const summaryLines = row.lines.map((line, index) => {
    const target = line.cartonNumber ? `整箱 ${line.cartonNumber}` : `${line.goodsCode ?? ''} ${line.unit ?? ''}`.trim();
    return {
      label: `第 ${index + 1} 行`,
      value: `${line.ownerAccount} · ${target} · 入账 ${line.posted}${line.shortOnBook > 0 ? ` · 账上不足 ${line.shortOnBook}` : ''}${line.held > 0 ? ` · 占用 ${line.held}` : ''}${line.cartonMissing ? ' · 缺箱' : ''}`,
    };
  });
  return {
    title: row.factId,
    subtitle: jobKindLabels[row.kind] ?? row.kind,
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '仓', value: row.warehouse, mono: true },
          { label: '设备序号', value: String(row.deviceSequence), mono: true },
          { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
          { label: '入册时刻', value: formatInstant(row.receivedAt), mono: true },
          ...summaryLines,
        ],
      },
    ],
  };
}

function InternalJobsTable() {
  const [keyword, setKeyword] = useState('');
  const [kind, setKind] = useState<JobKindFilter>('ALL');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = {
    q: settled,
    filters: kind === 'ALL' ? undefined : { kind: [kind] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listInternalJobs);
  const rows = answer?.kind === 'outcome' ? answer.body.jobs : [];

  return (
    <ListPageTemplate<InternalJobRecord>
      moduleId="warehouse-internal-ops"
      title={info.title}
      description="已入册的库内作业事实。上架、移库、转换与短少由一线作业端交来；差异批准、盘点、冻结、指令和库存调整在「处理」签。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜事实标识或仓编码' }}
      filters={
        <>
          {(['ALL', 'INTERNAL_MOVE', 'FORM_CONVERSION', 'CONDITION_CONVERSION', 'SHORTAGE'] as const).map((item) => (
            <button
              key={item}
              type="button"
              className={chipClass(item === kind)}
              aria-pressed={item === kind}
              onClick={() => setKind(item)}
            >
              {item === 'ALL' ? '全部' : jobKindLabels[item]}
            </button>
          ))}
        </>
      }
      columns={jobColumns}
      rows={rows}
      rowKey={(row) => row.factId}
      inspector={jobInspector}
      pagination={pagination}
      emptyRowsNote="当前条件下没有匹配的库内作业"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-internal-jobs',
        emptyTitle: '还没有库内作业记录',
        emptyDescription: '作业端登记上架、移库、形态转换、状态转换或短少观察后，会在这里出现。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

function StockLookup() {
  const [owner, setOwner] = useState('');
  const [warehouse, setWarehouse] = useState('');
  const [view, setView] = useState<StockViewKind>('cells');
  const [loading, setLoading] = useState(false);
  const [answer, setAnswer] = useState<ApiResult<StockReadBody> | null>(null);

  const run = () => {
    if (owner.trim() === '') return;
    setLoading(true);
    void queryOwnerStock(owner.trim(), warehouse.trim(), view).then((result) => {
      setAnswer(result);
      setLoading(false);
    });
  };

  return (
    <div className="flex-1 overflow-auto p-4 space-y-4">
      <div className="flex flex-wrap items-end gap-2">
        <Input
          className="w-56"
          value={owner}
          onChange={(event) => setOwner(event.target.value)}
          placeholder="货主客户账户（必填）"
        />
        <Input
          className="w-44"
          value={warehouse}
          onChange={(event) => setWarehouse(event.target.value)}
          placeholder="仓编码（可选）"
        />
        <div className="flex flex-wrap gap-1.5">
          {(['cells', 'cartons', 'summary', 'holds', 'movements'] as const).map((item) => (
            <button key={item} type="button" className={chipClass(item === view)} onClick={() => setView(item)}>
              {stockViewLabels[item]}
            </button>
          ))}
        </div>
        <Button type="button" disabled={loading || owner.trim() === ''} onClick={run}>
          {loading ? '查询中…' : '查询库存'}
        </Button>
      </div>

      {answer === null ? (
        <p className="type-body text-idpxyz-textMuted">输入货主客户账户并选择视图后查询在库、整箱、汇总、占用或库存流水。</p>
      ) : answer.kind !== 'outcome' ? (
        <p className="type-body text-idpxyz-danger">
          {answer.kind === 'callerProblem'
            ? `${problemNote(answer.code)}${answer.detail ? ` ${answer.detail}` : ''}`
            : answer.kind === 'noAnswer'
              ? problemNote(answer.code)
              : answer.kind === 'transport'
                ? answer.message
                : '当前登录身份无权查阅或接入渠道未配置。'}
        </p>
      ) : (
        <pre className="p-3 rounded border border-idpxyz-border bg-idpxyz-panel overflow-auto font-mono type-body leading-5">
          {JSON.stringify(answer.body, null, 2)}
        </pre>
      )}
    </div>
  );
}

/** 库内作业与盘点页：作业列表、库存读口与库内运营决定面板。 */
export function WarehouseInternalOpsPage() {
  const targets = useWarehouseActionTargets(internalOpsActionSpecs);

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="jobs" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="jobs">库内作业</TabsTrigger>
          <TabsTrigger value="stock">库存查阅</TabsTrigger>
          {targets.length > 0 ? <TabsTrigger value="act">处理</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="jobs" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <InternalJobsTable />
        </TabsContent>
        <TabsContent value="stock" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <StockLookup />
        </TabsContent>
        {targets.length > 0 ? (
          <TabsContent value="act" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <WarehouseActionPanel moduleId="warehouse-internal-ops" targets={targets} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
