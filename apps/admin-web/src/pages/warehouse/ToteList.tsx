import { useState } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent, InspectorField } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listTotes, type ToteRecord } from './api';
import { DocumentLink } from './document-page';
import { canRetireTote, liveToteStates, toteStateBadge, toteStateLabel } from './tote';
import { openDocument } from './use-document';
import { useStatusCounts } from './use-status-counts';
import { WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const moduleId = 'warehouse-waves';
const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

/**
 * 周转箱名册：每个仓登记过哪些箱，此刻各自空着、绑着哪张出库单，还是已停用。
 * 拣货车上的格口也在这里按箱登记。扫到没登记的码拣货不认，所以箱要先登在这里。
 */
export function ToteList() {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const [keyword, setKeyword] = useState('');
  const [warehouse, setWarehouse] = useState('');
  const [state, setState] = useState('ALL');
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(keyword);
  const base: CatalogueConditions = { q: settled, filters: warehouse ? { warehouseCode: [warehouse] } : {} };
  const conditions: CatalogueConditions = state === 'ALL' ? base : { ...base, filters: { ...base.filters, state: [state] } };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listTotes);
  const rows = answer?.kind === 'outcome' ? answer.body.totes : [];
  const counts = useStatusCounts(listTotes, base, 'state', liveToteStates, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };

  const columns: ListColumn<ToteRecord>[] = [
    { id: 'code', header: '箱号', className: mono, render: (row) => row.toteCode },
    { id: 'state', header: '状态', render: (row) => <WmsBadge code={toteStateBadge(row.state)} label={toteStateLabel(row.state)} /> },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    {
      id: 'request',
      header: '绑着的出库单',
      className: 'whitespace-nowrap',
      render: (row) => (row.requestId ? <DocumentLink moduleId="warehouse-outbound" parts={[row.requestId]} /> : '—'),
    },
    { id: 'bound', header: '绑上时刻', className: mono, render: (row) => (row.boundAt ? formatInstant(row.boundAt) : '—') },
    { id: 'registered', header: '登记时刻', className: mono, render: (row) => formatInstant(row.registeredAt) },
  ];

  const inspectorOf = (row: ToteRecord): InspectorContent => {
    const actions: InspectorAction[] = [];
    if (row.requestId) {
      const requestId = row.requestId;
      actions.push({ label: '打开绑着的出库单', onRun: () => openDocument('warehouse-outbound', [requestId]) });
    }
    if (row.state === 'IN_USE' && drawer.has('tote-release')) {
      actions.push({ label: '放开这个箱', onRun: () => drawer.open('tote-release', { warehouse: row.warehouseCode, toteCode: row.toteCode }) });
    }
    if (canRetireTote(row.state) && drawer.has('tote-retire')) {
      actions.push({ label: '停用这个箱', onRun: () => drawer.open('tote-retire', { warehouse: row.warehouseCode, toteCode: row.toteCode }) });
    }
    if (row.state === 'RETIRED' && drawer.has('tote-register')) {
      actions.push({ label: '恢复在用', onRun: () => drawer.open('tote-register', { warehouse: row.warehouseCode, toteCodes: row.toteCode }) });
    }
    const audit: InspectorField[] = [{ label: '登记', value: `${row.registeredBy} · ${formatInstant(row.registeredAt)}`, mono: true }];
    if (row.retiredAt) audit.push({ label: '停用', value: `${row.retiredBy ?? ''} · ${formatInstant(row.retiredAt)}`, mono: true });
    return {
      title: row.toteCode,
      subtitle: row.warehouseCode,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '绑着的出库单', value: row.requestId ?? '没有', mono: true },
            { label: '绑上时刻', value: row.boundAt ? formatInstant(row.boundAt) : '—', mono: true },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: toteStateLabel(row.state) }] },
        { kind: 'actions', actions },
        { kind: 'audit', fields: audit },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<ToteRecord>
        moduleId={moduleId}
        title="周转箱"
        description="每个仓登记过的周转箱，和它此刻空着、绑着哪张出库单。拣货车上的格口也在这里按箱登记。拣货只认登记过的箱号。"
        headerActions={
          drawer.has('tote-register') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('tote-register', warehouse ? { warehouse } : undefined)}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              登记周转箱
            </Button>
          ) : undefined
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜箱号或绑着的出库请求号' }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={state}
              onChange={setState}
              tabs={[
                ...liveToteStates.map((code) => ({ value: code, label: toteStateLabel(code), count: counts[code] })),
                { value: 'RETIRED', label: toteStateLabel('RETIRED') },
                { value: 'ALL', label: '全部' },
              ]}
            />
            <WmsFilterSelect
              label="仓库"
              value={warehouse}
              onChange={setWarehouse}
              options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: `${w.warehouseCode} · ${w.name}` }))}
            />
          </div>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => `${row.warehouseCode}/${row.toteCode}`}
        inspector={inspectorOf}
        pagination={pagination}
        emptyRowsNote="当前筛选下没有周转箱"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-totes',
          emptyTitle: '还没有登记周转箱',
          emptyDescription: '点右上角「登记周转箱」，按仓一次登一批箱号。拣货车上的格口也按箱登记。',
          narrowed: settled.trim() !== '' || warehouse !== '' || state !== 'ALL',
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}
