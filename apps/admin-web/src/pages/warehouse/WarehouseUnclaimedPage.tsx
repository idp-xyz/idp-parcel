import { useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent, InspectorField } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { listUnclaimedArrivals, type UnclaimedArrivalRecord } from './api';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';
import { UnclaimedDetailModal } from './document-modals';
import { WmsBadge } from './wms-ui';

const info = moduleInfoById['warehouse-unclaimed'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

type ArrivalStatusFilter = 'ALL' | 'UNCLAIMED' | 'CLAIMED' | 'DISPOSED';

const statusLabels: Record<string, string> = {
  UNCLAIMED: '待认领',
  CLAIMED: '已认领',
  DISPOSED: '已处置',
};

const kindLabels: Record<string, string> = {
  INBOUND: '入库到货',
  RETURN: '退件',
};

const columns: ListColumn<UnclaimedArrivalRecord>[] = [
  { id: 'id', header: '到货号', className: mono, render: (row) => row.arrivalId },
  { id: 'warehouse', header: '仓库', className: mono, render: (row) => row.warehouseCode },
  { id: 'kind', header: '种类', render: (row) => kindLabels[row.kind] ?? row.kind },
  { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} label={statusLabels[row.status]} /> },
  { id: 'obs', header: '现场观察', render: (row) => row.observation || '—' },
  { id: 'received', header: '登记时刻', className: mono, render: (row) => formatInstant(row.receivedAt) },
];

/** 待认领到货专属队列页：聚焦认领、撤销认领与按规则处置，处置规则配置已归到「仓配规则设置」。 */
export function WarehouseUnclaimedPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(receivingActionSpecs);

  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<ArrivalStatusFilter>('UNCLAIMED');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [opened, setOpened] = useState<UnclaimedArrivalRecord | null>(null);
  const settled = useSettledKeyword(keyword);

  const conditions: CatalogueConditions = {
    q: settled,
    filters: status === 'ALL' ? undefined : { status: [status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listUnclaimedArrivals);
  const rows = answer?.kind === 'outcome' ? answer.body.arrivals : [];

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const inspectorOf = (row: UnclaimedArrivalRecord): InspectorContent => {
    const fields: InspectorField[] = [
      { label: '事实标识', value: row.factId, mono: true },
      { label: '仓库 / 来源', value: `${row.warehouseCode} · ${row.origin}`, mono: true },
      { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
      { label: '观察', value: row.observation || '—' },
      { label: '货主 / 预报线索', value: `${row.ownerClue || '—'} / ${row.preAdviceClue || '—'}`, mono: true },
      { label: '存放处', value: row.storageLocation || '—', mono: true },
    ];
    if (row.disposition) {
      fields.push(
        { label: '处置方式与规则', value: `${row.disposition.method}（${row.disposition.ruleWarehouseCode} r${row.disposition.ruleRevision}）` },
        { label: '处置时刻', value: formatInstant(row.disposition.disposedAt), mono: true },
      );
    }

    const actions: InspectorAction[] = [{ label: '查看到货明细', onRun: () => setOpened(row) }];
    if (row.status === 'UNCLAIMED') {
      if (targetMap.has('unclaimed-claim')) {
        actions.push({
          label: '认领这件到货',
          onRun: () =>
            openAction('unclaimed-claim', {
              arrivalId: row.arrivalId,
              ownerAccount: row.ownerClue ?? '',
              preAdviceId: row.preAdviceClue ?? '',
            }),
        });
      }
      if (targetMap.has('unclaimed-dispose')) {
        actions.push({
          label: '按规则处置',
          onRun: () => openAction('unclaimed-dispose', { arrivalId: row.arrivalId }),
        });
      }
    } else if (row.status === 'CLAIMED' && targetMap.has('unclaimed-revoke')) {
      actions.push({
        label: '撤销认领',
        onRun: () => openAction('unclaimed-revoke', { arrivalId: row.arrivalId }),
      });
    }

    return {
      title: row.arrivalId,
      subtitle: statusLabels[row.status] ?? row.status,
      sections: [
        { kind: 'summary', fields },
        { kind: 'status', items: [{ label: '状态', word: statusLabels[row.status] ?? row.status }] },
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
        { kind: 'audit', fields: [{ label: '操作者 / 设备', value: `${row.operator} · ${row.device}`, mono: true }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<UnclaimedArrivalRecord>
        moduleId="warehouse-unclaimed"
        title={info.title}
        description="无主到货与逾期到达。双击看现场观察和线索；单击行在右栏认领或按规则处置。"
        headerActions={
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-settings'; }}>
              管理处置规则
            </Button>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
              去作业台登记到货
            </Button>
          </div>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜到货号、仓或观察描述' }}
        filters={
          <>
            {(['UNCLAIMED', 'CLAIMED', 'DISPOSED', 'ALL'] as const).map((item) => (
              <button
                key={item}
                type="button"
                className={chipClass(item === status)}
                aria-pressed={item === status}
                onClick={() => setStatus(item)}
              >
                {item === 'ALL' ? '全部' : statusLabels[item]}
              </button>
            ))}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.arrivalId}
        inspector={inspectorOf}
        onRowOpen={setOpened}
        pagination={pagination}
        emptyRowsNote="当前条件下没有匹配的待认领到货"
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-unclaimed-arrivals',
          emptyTitle: status === 'UNCLAIMED' ? '没有待认领到货' : '还没有待认领记录',
          emptyDescription: '在「仓内作业台」登记无主到货，或预报取消/完结后再收货时，记录会出现在这里。',
          narrowed: settled.trim() !== '' || (status !== 'UNCLAIMED' && status !== 'ALL'),
        })}
      />
      <UnclaimedDetailModal row={opened} onClose={() => setOpened(null)} />
      <ActionDrawer
        moduleId="warehouse-unclaimed"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => retry()}
      />
    </>
  );
}
