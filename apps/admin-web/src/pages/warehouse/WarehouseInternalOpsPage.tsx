import { useState } from 'react';
import { Button, Input, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent, InspectorField } from '../../templates/inspector';
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
import type { GuidedFormSeed, GuidedFormValues } from './guided-forms';
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

export function buildInternalJobInspector(
  row: InternalJobRecord,
  available: ReadonlySet<string>,
  onPrefill: (targetId: string, values: GuidedFormValues) => void,
): InspectorContent {
  // 概要严格不超过 8 格（INSPECTOR_SUMMARY_LIMIT）：头部 4 格 + 最多 4 格明细（超出时第 4 格汇总剩余行数）。
  const fields: InspectorField[] = [
    { label: '仓', value: row.warehouse, mono: true },
    { label: '设备序号', value: String(row.deviceSequence), mono: true },
    { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
    { label: '入册时刻', value: formatInstant(row.receivedAt), mono: true },
  ];
  const visible = row.lines.length > 4 ? row.lines.slice(0, 3) : row.lines;
  visible.forEach((line, index) => {
    const target = line.cartonNumber ? `整箱 ${line.cartonNumber}` : `${line.goodsCode ?? ''} ${line.unit ?? ''}`.trim();
    fields.push({
      label: `第 ${index + 1} 行`,
      value: `${line.ownerAccount} · ${target} · 入账 ${line.posted}${line.shortOnBook > 0 ? ` · 不足 ${line.shortOnBook}` : ''}${line.held > 0 ? ` · 占用 ${line.held}` : ''}${line.cartonMissing ? ' · 缺箱' : ''}`,
    });
  });
  if (row.lines.length > 4) {
    fields.push({ label: '其余明细', value: `另有 ${row.lines.length - 3} 行未展开` });
  }

  const first = row.lines[0];
  const actions: InspectorAction[] = [];
  if (first && available.has('condition-restore')) {
    actions.push({
      label: '次品转良品',
      onRun: () =>
        onPrefill('condition-restore', {
          warehouse: row.warehouse,
          ownerAccount: first.ownerAccount,
          locationCode: first.targetLocation ?? first.sourceLocation ?? '',
          goodsCode: first.goodsCode ?? '',
          unit: first.unit ?? '',
          stockState: first.stockState ?? '',
        }),
    });
  }
  if (first && available.has('stock-freeze')) {
    actions.push({
      label: '冻结相关库存',
      onRun: () =>
        onPrefill('stock-freeze', {
          warehouse: row.warehouse,
          ownerAccount: first.ownerAccount,
          cartonNumber: first.cartonNumber ?? '',
          locationCode: first.cartonNumber ? '' : first.targetLocation ?? first.sourceLocation ?? '',
          goodsCode: first.cartonNumber ? '' : first.goodsCode ?? '',
          unit: first.cartonNumber ? '' : first.unit ?? '',
          stockState: first.cartonNumber ? '' : first.stockState ?? '',
        }),
    });
  }
  if (available.has('count-create')) {
    const locations = [...new Set(row.lines.map((line) => line.targetLocation ?? line.sourceLocation ?? '').filter(Boolean))];
    actions.push({
      label: '对涉及库位建盘点',
      onRun: () =>
        onPrefill('count-create', {
          warehouse: row.warehouse,
          ownerAccount: first?.ownerAccount ?? '',
          locations: locations.join(', '),
        }),
    });
  }

  return {
    title: row.factId,
    subtitle: jobKindLabels[row.kind] ?? row.kind,
    sections: [
      { kind: 'summary', fields },
      ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
    ],
  };
}

function InternalJobsTable({
  available,
  onPrefill,
}: {
  available: ReadonlySet<string>;
  onPrefill: (targetId: string, values: GuidedFormValues) => void;
}) {
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
      description="已入册的库内作业事实。单击行可从右栏带出仓、货主与库位发起转良品、冻结或盘点；全部库内决定在「处理」签。"
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
      inspector={(row) => buildInternalJobInspector(row, available, onPrefill)}
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

function StockLookup({
  available,
  onPrefill,
}: {
  available: ReadonlySet<string>;
  onPrefill: (targetId: string, values: GuidedFormValues) => void;
}) {
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
      ) : view === 'cells' && answer.body.cells ? (
        answer.body.cells.length === 0 ? (
          <p className="type-body text-idpxyz-textMuted">当前条件下没有在库商品格。</p>
        ) : (
          <div className="overflow-auto border border-idpxyz-border rounded">
            <table className="w-full text-left type-body">
              <thead className="border-b border-idpxyz-border text-idpxyz-textMuted">
                <tr>
                  <th className="px-3 py-2">仓 / 库位</th>
                  <th className="px-3 py-2">商品 / 单位</th>
                  <th className="px-3 py-2">状态</th>
                  <th className="px-3 py-2">入库批次</th>
                  <th className="px-3 py-2">数量 / 冻结 / 可用</th>
                  <th className="px-3 py-2">库龄</th>
                  <th className="px-3 py-2">快捷处理</th>
                </tr>
              </thead>
              <tbody>
                {answer.body.cells.map((cell, index) => (
                  <tr key={`${cell.warehouse}/${cell.location ?? ''}/${cell.inboundBatchId}/${index}`} className="border-b border-idpxyz-border last:border-b-0">
                    <td className="px-3 py-2 font-mono">{`${cell.warehouse} · ${cell.location ?? '—'}`}</td>
                    <td className="px-3 py-2 font-mono">{`${cell.goodsCode} (${cell.unit})`}</td>
                    <td className="px-3 py-2">{cell.stockState}</td>
                    <td className="px-3 py-2 font-mono">{cell.inboundBatchId}</td>
                    <td className="px-3 py-2 font-mono">{`${cell.quantity} / ${cell.frozen} / ${cell.available}`}</td>
                    <td className="px-3 py-2 font-mono">{cell.ageDays !== undefined ? `${cell.ageDays} 天` : cell.age ?? '—'}</td>
                    <td className="px-3 py-2">
                      {available.has('stock-freeze') && cell.location ? (
                        <Button
                          type="button"
                          variant="outline"
                          onClick={() =>
                            onPrefill('stock-freeze', {
                              ownerAccount: owner.trim(),
                              warehouse: cell.warehouse,
                              locationCode: cell.location ?? '',
                              goodsCode: cell.goodsCode,
                              unit: cell.unit,
                              stockState: cell.stockState,
                              inboundBatchId: cell.inboundBatchId,
                              quantity: String(cell.available > 0 ? cell.available : cell.quantity),
                            })
                          }
                        >
                          冻结此格
                        </Button>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      ) : (
        <pre className="p-3 rounded border border-idpxyz-border bg-idpxyz-panel overflow-auto font-mono type-body leading-5">
          {JSON.stringify(answer.body, null, 2)}
        </pre>
      )}
    </div>
  );
}

/** 库内作业与盘点页：作业列表、库存读口与库内引导式运营决定面板。 */
export function WarehouseInternalOpsPage() {
  const targets = useWarehouseActionTargets(internalOpsActionSpecs);
  const available = new Set(targets.map((target) => target.id));
  const [tab, setTab] = useState('jobs');
  const [seed, setSeed] = useState<GuidedFormSeed | null>(null);

  const openPrefill = (targetId: string, values: GuidedFormValues) => {
    setSeed((current) => ({ targetId, values, nonce: (current?.nonce ?? 0) + 1 }));
    setTab('act');
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="jobs">库内作业</TabsTrigger>
          <TabsTrigger value="stock">库存查阅</TabsTrigger>
          {targets.length > 0 ? <TabsTrigger value="act">处理</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="jobs" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <InternalJobsTable available={available} onPrefill={openPrefill} />
        </TabsContent>
        <TabsContent value="stock" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <StockLookup available={available} onPrefill={openPrefill} />
        </TabsContent>
        {targets.length > 0 ? (
          <TabsContent value="act" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <WarehouseActionPanel moduleId="warehouse-internal-ops" targets={targets} seed={seed} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
