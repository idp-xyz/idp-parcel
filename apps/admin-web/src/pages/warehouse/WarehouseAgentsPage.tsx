import { useState } from 'react';
import { Plus, Settings } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { listOutboundRequests, type OutboundRequestRecord } from './api';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';

const info = moduleInfoById['warehouse-agents'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const statusLabels: Record<string, string> = {
  HANDED_OFF: '待转出代理仓',
  FORWARDED: '已转出代理仓',
  PACKED: '代理仓已打包',
  SHIPPED: '代理仓已交接',
};

const agentStatusFilters = ['HANDED_OFF', 'FORWARDED', 'PACKED', 'SHIPPED'] as const;

const columns: ListColumn<OutboundRequestRecord>[] = [
  { id: 'id', header: '出库单号', className: mono, render: (row) => row.documentNumber ?? row.requestId },
  { id: 'warehouse', header: '代理仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
  { id: 'status', header: '履约进展', render: (row) => statusLabels[row.status] ?? row.status },
  { id: 'recipient', header: '收件人', render: (row) => row.recipient },
  { id: 'received', header: '受理时刻', className: mono, render: (row) => formatInstant(row.receivedAt) },
];

/** 代理仓履约页：把代理仓外发、回执（打包/交接/拒单）与库存声明独立成清晰队列，不再和自营仓波次混在一页。 */
export function WarehouseAgentsPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(outboundWaveActionSpecs);

  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<string>('HANDED_OFF');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const settled = useSettledKeyword(keyword);

  const conditions: CatalogueConditions = {
    q: settled,
    filters: { status: [status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listOutboundRequests);
  const agentWarehouseCodes = new Set(lookups.warehouses.filter((w) => w.kind === 'AGENT').map((w) => w.warehouseCode));
  const rows =
    answer?.kind === 'outcome'
      ? answer.body.requests.filter(
          (r) => r.status === 'HANDED_OFF' || r.status === 'FORWARDED' || agentWarehouseCodes.has(r.warehouseCode),
        )
      : [];

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const inspectorOf = (row: OutboundRequestRecord): InspectorContent => {
    const actions: InspectorAction[] = [];
    if (row.status === 'HANDED_OFF' && targetMap.has('agent-result')) {
      actions.push({ label: '转出给代理仓', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'FORWARDED' }) });
    }
    if (row.status === 'FORWARDED' && targetMap.has('agent-result')) {
      actions.push(
        { label: '登记代理仓打包', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'PACKED' }) },
        { label: '登记代理仓交接出库', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'HANDED_OVER' }) },
        { label: '登记代理仓拒单', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'REJECTED' }) },
        { label: '登记代理仓缺货', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'SHORT' }) },
      );
    }
    if (row.status === 'PACKED' && targetMap.has('agent-result')) {
      actions.push({ label: '登记代理仓交接出库', onRun: () => openAction('agent-result', { requestId: row.requestId, kind: 'HANDED_OVER' }) });
    }

    return {
      title: row.documentNumber ?? row.requestId,
      subtitle: `${row.warehouseCode} · ${statusLabels[row.status] ?? row.status}`,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '出库请求号', value: row.requestId, mono: true },
            { label: '货主', value: row.ownerAccount, mono: true },
            { label: '收件人', value: row.recipient },
            { label: '收件地址', value: row.address },
            { label: '需求数量', value: `${row.wantedQuantity}（${row.lines.length} 行）`, mono: true },
          ],
        },
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
        { kind: 'audit', fields: [{ label: '受理时刻', value: formatInstant(row.receivedAt), mono: true }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<OutboundRequestRecord>
        moduleId="warehouse-agents"
        title={info.title}
        description="代理仓不走自营波次与拣货车：在此转出指令、回录打包/交接/拒单，以及登记代理仓库存声明。代理仓连接器在「仓配规则设置」管理。"
        headerActions={
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                window.location.hash = '#/warehouse-settings';
              }}
            >
              <Settings className="h-3.5 w-3.5 mr-1" />
              配置代理仓连接器
            </Button>
            {targetMap.has('agent-statement') ? (
              <Button type="button" className={primaryActionClass} onClick={() => openAction('agent-statement')}>
                <Plus className="h-3.5 w-3.5 mr-1" />
                登记代理仓库存声明
              </Button>
            ) : null}
          </div>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜单号、代理仓或货主' }}
        filters={
          <>
            {agentStatusFilters.map((item) => (
              <button
                key={item}
                type="button"
                className={chipClass(item === status)}
                aria-pressed={item === status}
                onClick={() => setStatus(item)}
              >
                {statusLabels[item]}
              </button>
            ))}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.requestId}
        inspector={inspectorOf}
        pagination={pagination}
        emptyRowsNote="当前状态下没有代理仓履约订单"
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-outbound-request-list',
          emptyTitle: '当前状态没有代理仓订单',
          emptyDescription: '向类型为「代理仓」的仓库下出库单后，订单会直接进入「待转出代理仓」队列。',
          narrowed: settled.trim() !== '',
        })}
      />
      <ActionDrawer
        moduleId="warehouse-agents"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => retry()}
      />
    </>
  );
}
