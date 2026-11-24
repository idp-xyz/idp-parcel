import { useMemo, useState } from 'react';
import { Plus, Send } from 'lucide-react';
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

const info = moduleInfoById['warehouse-outbound'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const statusLabels: Record<string, string> = {
  RESERVED: '已预占',
  PARTIALLY_RESERVED: '部分预占',
  PENDING_POLICY: '待定缺货策略',
  ALLOCATED: '已放波次',
  PICKED: '已拣货',
  CHECKED: '已复核',
  PACKED: '已打包',
  SHIPPED: '已出库',
  SPLIT: '已拆单',
  REJECTED: '已拒单',
  CANCELLED: '已取消',
  HANDED_OFF: '待代理仓处理',
  FORWARDED: '已转出代理仓',
};

const statusFilters = ['ALL', 'RESERVED', 'ALLOCATED', 'PICKED', 'CHECKED', 'PACKED', 'SHIPPED', 'PARTIALLY_RESERVED', 'CANCELLED'] as const;

const columns: ListColumn<OutboundRequestRecord>[] = [
  { id: 'id', header: '出库单号', className: mono, render: (row) => row.documentNumber ?? row.requestId },
  { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'status', header: '状态', render: (row) => statusLabels[row.status] ?? row.status },
  { id: 'dispatch', header: '发货途径', render: (row) => (row.dispatch === 'OWNER_LABEL' ? '货主面单' : row.service || '走发运') },
  { id: 'qty', header: '预占 / 需求', className: mono, render: (row) => `${row.reservedQuantity} / ${row.wantedQuantity}` },
  { id: 'received', header: '受理时刻', className: mono, render: (row) => formatInstant(row.receivedAt) },
];

/** 出库订单页：支持勾选多张「已预占」订单一键合并释放波次；右上角代录出库单；右栏直接单票放波次、拆单、截单取消与登记货主面单。 */
export function WarehouseOutboundPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(outboundWaveActionSpecs);

  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<string>('ALL');
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);

  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = {
    q: settled,
    filters: status === 'ALL' ? undefined : { status: [status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listOutboundRequests);
  const rows = answer?.kind === 'outcome' ? answer.body.requests : [];

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const selectedRecords = useMemo(() => rows.filter((r) => selected.has(r.requestId)), [rows, selected]);
  const selectedWarehouses = useMemo(() => new Set(selectedRecords.map((r) => r.warehouseCode)), [selectedRecords]);
  const allSelectedReserved = selectedRecords.length > 0 && selectedRecords.every((r) => r.status === 'RESERVED');
  const canBulkWave =
    targetMap.has('wave-release') && selectedRecords.length > 0 && selectedWarehouses.size === 1 && allSelectedReserved;

  const bulkWaveButton = targetMap.has('wave-release') ? (
    <Button
      type="button"
      size="sm"
      className={primaryActionClass}
      disabled={!canBulkWave}
      title={
        canBulkWave
          ? '将所选已预占订单合并为一个波次释放'
          : '仅支持勾选同一仓库、状态为「已预占」的出库单合并放波次'
      }
      onClick={() => {
        const warehouse = selectedRecords[0]?.warehouseCode ?? '';
        const requests = selectedRecords.map((r) => r.requestId).join(', ');
        openAction('wave-release', { warehouse, requests });
      }}
    >
      <Send className="h-3.5 w-3.5 mr-1" />
      {`合并释放波次（${selectedRecords.length} 单）`}
    </Button>
  ) : undefined;

  const inspectorOf = (row: OutboundRequestRecord): InspectorContent => {
    const actions: InspectorAction[] = [];
    if (row.status === 'RESERVED' && targetMap.has('wave-release')) {
      actions.push({
        label: '单票释放波次',
        onRun: () => openAction('wave-release', { warehouse: row.warehouseCode, requests: row.requestId }),
      });
    }
    if ((row.status === 'RESERVED' || row.status === 'PARTIALLY_RESERVED' || row.status === 'PENDING_POLICY') && targetMap.has('outbound-split')) {
      actions.push({
        label: '拆分出库单',
        onRun: () => openAction('outbound-split', { requestId: row.requestId }),
      });
    }
    if (row.status !== 'SHIPPED' && row.status !== 'CANCELLED' && targetMap.has('outbound-cutoff')) {
      actions.push({
        label: '截单取消',
        onRun: () => openAction('outbound-cutoff', { requestId: row.requestId }),
      });
    }
    if (row.dispatch === 'OWNER_LABEL' && targetMap.has('owner-label')) {
      actions.push({
        label: '登记货主面单',
        onRun: () => openAction('owner-label', { requestId: row.requestId }),
      });
    }
    if (row.status === 'ALLOCATED' || row.status === 'PICKED' || row.status === 'CHECKED' || row.status === 'PACKED') {
      actions.push({
        label: '去作业台执行下一步',
        onRun: () => {
          window.location.hash = '#/warehouse-workstation';
        },
      });
    }

    return {
      title: row.documentNumber ?? row.requestId,
      subtitle: `${row.requestId} · ${statusLabels[row.status] ?? row.status}`,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主 / 仓库', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
            { label: '发货途径', value: row.dispatch === 'OWNER_LABEL' ? '货主面单' : row.service || '走发运' },
            { label: '收件人', value: row.recipient },
            { label: '收件地址', value: row.address },
            { label: '预占 / 需求', value: `${row.reservedQuantity} / ${row.wantedQuantity}（${row.lines.length} 行）`, mono: true },
            { label: '单据号状态', value: row.documentNumber ?? row.numberingAbsence ?? '—', mono: true },
            ...(row.prefetchShipmentId ? [{ label: '预取面单运单', value: row.prefetchShipmentId, mono: true }] : []),
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: statusLabels[row.status] ?? row.status }] },
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
        { kind: 'audit', fields: [{ label: '来源 / 受理时刻', value: `${row.origin} · ${formatInstant(row.receivedAt)}`, mono: true }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<OutboundRequestRecord>
        moduleId="warehouse-outbound"
        title={info.title}
        description="出库请求台账。勾选多张同仓「已预占」订单即可批量合并释放波次；单击行可拆单、截单或登记面单。"
        headerActions={
          <div className="flex flex-wrap gap-2">
            {targetMap.has('outbound-request') ? (
              <Button type="button" className={primaryActionClass} onClick={() => openAction('outbound-request')}>
                <Plus className="h-3.5 w-3.5 mr-1" />
                代录出库单
              </Button>
            ) : null}
          </div>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜出库单号、货主、仓或收件人' }}
        filters={
          <>
            {statusFilters.map((item) => (
              <button
                key={item}
                type="button"
                className={chipClass(item === status)}
                aria-pressed={item === status}
                onClick={() => setStatus(item)}
              >
                {item === 'ALL' ? '全部' : statusLabels[item] ?? item}
              </button>
            ))}
          </>
        }
        selection={{ selected, onChange: setSelected }}
        bulkActions={{ extra: bulkWaveButton }}
        columns={columns}
        rows={rows}
        rowKey={(row) => row.requestId}
        inspector={inspectorOf}
        pagination={pagination}
        emptyRowsNote="当前条件下没有匹配的出库单"
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-outbound-request-list',
          emptyTitle: '还没有出库订单',
          emptyDescription: '点右上角「代录出库单」创建测试或运营代录单，或等待货主经客户接口提交。',
          narrowed: settled.trim() !== '' || status !== 'ALL',
        })}
      />
      <ActionDrawer
        moduleId="warehouse-outbound"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => {
          setSelected(new Set());
          retry();
        }}
      />
    </>
  );
}
