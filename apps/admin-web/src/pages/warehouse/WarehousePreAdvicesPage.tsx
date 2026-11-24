import { useEffect, useState } from 'react';
import { Plus, RefreshCw } from 'lucide-react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { listOwnerPreAdvices, type ApiResult, type PreAdviceListBody, type PreAdviceRecord } from './api';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';
import { PreAdviceDetailModal } from './document-modals';
import { WmsBadge } from './wms-ui';

const info = moduleInfoById['warehouse-pre-advices'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const statusLabels: Record<string, string> = {
  ADVISED: '已预报',
  RECEIVING: '收货中',
  CLOSED: '已完结',
  CANCELLED: '已取消',
};

const inboundModeLabels: Record<string, string> = {
  BY_GOODS: '按商品',
  BY_CARTON: '按整箱',
};

const columns: ListColumn<PreAdviceRecord>[] = [
  { id: 'id', header: '预报标识', className: mono, render: (row) => row.preAdviceId },
  { id: 'number', header: '入库单号', className: mono, render: (row) => row.documentNumber ?? '—' },
  { id: 'warehouse', header: '仓库', className: mono, render: (row) => row.warehouseCode },
  { id: 'mode', header: '入库形态', render: (row) => inboundModeLabels[row.inboundMode] ?? row.inboundMode },
  { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} label={statusLabels[row.status]} /> },
  { id: 'at', header: '受理时刻', className: mono, render: (row) => formatInstant(row.acceptedAt) },
];

/** 入库预报管理页：按货主查阅入库预报单，页头直接代录预报与录差异结论，行内右栏直接更新告知、撤回、完结或跳转作业台收货。 */
export function WarehousePreAdvicesPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(receivingActionSpecs);

  const [owner, setOwner] = useState('');
  const [keyword, setKeyword] = useState('');
  const [answer, setAnswer] = useState<ApiResult<PreAdviceListBody> | null>(null);
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    if (owner === '' && lookups.owners.length > 0) {
      setOwner(lookups.owners[0].accountId);
    }
  }, [lookups.owners, owner]);

  useEffect(() => {
    const trimmed = owner.trim();
    if (!trimmed) {
      setAnswer(null);
      return;
    }
    let cancelled = false;
    setAnswer(null);
    void listOwnerPreAdvices(trimmed, keyword).then((res) => {
      if (!cancelled) setAnswer(res);
    });
    return () => {
      cancelled = true;
    };
  }, [owner, keyword, tick]);

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const rows = answer?.kind === 'outcome' ? answer.body.preAdvices : [];

  const inspectorOf = (row: PreAdviceRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '查看完整明细', onRun: () => setDetailId(row.preAdviceId) }];
    if (row.status === 'ADVISED' || row.status === 'RECEIVING') {
      actions.push({
        label: '去作业台扫码收货',
        onRun: () => {
          window.location.hash = '#/warehouse-workstation';
        },
      });
    }
    if (targetMap.has('pre-advice-notice') && row.status !== 'CLOSED' && row.status !== 'CANCELLED') {
      actions.push({
        label: '更新到货告知',
        onRun: () => openAction('pre-advice-notice', { preAdviceId: row.preAdviceId }),
      });
    }
    if (targetMap.has('pre-advice-withdrawal') && row.status === 'ADVISED') {
      actions.push({
        label: '撤回预报',
        onRun: () => openAction('pre-advice-withdrawal', { preAdviceId: row.preAdviceId }),
      });
    }
    if (targetMap.has('pre-advice-close') && row.status === 'RECEIVING') {
      actions.push({
        label: '完结预报并计算少收',
        onRun: () => openAction('pre-advice-close', { preAdviceId: row.preAdviceId }),
      });
    }
    return {
      title: row.documentNumber ?? row.preAdviceId,
      subtitle: row.preAdviceId,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主', value: row.ownerAccount, mono: true },
            { label: '入库仓库', value: row.warehouseCode, mono: true },
            { label: '到仓 / 入库方式', value: `${row.arrivalMode} · ${inboundModeLabels[row.inboundMode] ?? row.inboundMode}` },
            { label: '商品行 / 整箱数', value: `${row.goodsLines?.length ?? 0} / ${row.cartons?.length ?? 0}`, mono: true },
            { label: '单据号状态', value: row.documentNumber ?? row.numberingAbsence ?? '—', mono: true },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: statusLabels[row.status] ?? row.status }] },
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
        {
          kind: 'audit',
          fields: [
            { label: '提交途径 / 登记人', value: `${row.submissionPath} · ${row.registrant}`, mono: true },
            { label: '受理时刻', value: formatInstant(row.acceptedAt), mono: true },
          ],
        },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<PreAdviceRecord>
        moduleId="warehouse-pre-advices"
        title={info.title}
        description="管理货主入库预报。双击看商品行、整箱、已收进度和收货事实；收货本身在作业台完成。"
        headerActions={
          <div className="flex flex-wrap items-center gap-2">
            <Button type="button" variant="outline" onClick={() => setTick((n) => n + 1)}>
              <RefreshCw className="h-3.5 w-3.5 mr-1" />
              刷新
            </Button>
            {targetMap.has('discrepancy-conclusion') ? (
              <Button type="button" variant="outline" onClick={() => openAction('discrepancy-conclusion')}>
                录收货差异结论
              </Button>
            ) : null}
            {targetMap.has('pre-advice-on-behalf') ? (
              <Button
                type="button"
                className={primaryActionClass}
                onClick={() => openAction('pre-advice-on-behalf', { ownerAccount: owner.trim() })}
              >
                <Plus className="h-3.5 w-3.5 mr-1" />
                代录入库预报
              </Button>
            ) : null}
          </div>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜预报号或仓编码' }}
        filters={
          <div className="flex items-center gap-2">
            {lookups.owners.length > 0 ? (
              <select
                className="h-6 rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-caption text-idpxyz-text"
                value={lookups.owners.some((o) => o.accountId === owner) ? owner : ''}
                onChange={(event) => {
                  if (event.target.value) setOwner(event.target.value);
                }}
              >
                <option value="">选择货主客户账户…</option>
                {lookups.owners.map((acct) => (
                  <option key={acct.accountId} value={acct.accountId}>
                    {acct.customerPartyName ? `${acct.accountId} · ${acct.customerPartyName}` : acct.accountId}
                  </option>
                ))}
              </select>
            ) : null}
            <Input
              className="h-6 w-48 font-mono type-caption"
              value={owner}
              onChange={(event) => setOwner(event.target.value)}
              placeholder="输入货主账户编码"
            />
          </div>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.preAdviceId}
        inspector={inspectorOf}
        onRowOpen={(row) => setDetailId(row.preAdviceId)}
        emptyRowsNote="该货主名下没有匹配的入库预报"
        viewState={
          owner.trim() === ''
            ? {
                kind: 'empty',
                title: '请先选择货主客户账户',
                description: '入库预报按货主归属。选择货主后即可查看并操作该货主的预报单。',
              }
            : catalogueViewState(answer, rows.length, () => setTick((n) => n + 1), {
                module: info,
                endpoint: 'GET /warehouse-fulfillment-inbound-pre-advices',
                emptyTitle: '该货主还没有入库预报',
                emptyDescription: '点右上角「代录入库预报」新建，或等待货主经客户接口提交。',
                narrowed: keyword.trim() !== '',
              })
        }
      />
      <PreAdviceDetailModal
        ownerAccount={owner.trim()}
        preAdviceId={detailId}
        canNotice={targetMap.has('pre-advice-notice')}
        canWithdraw={targetMap.has('pre-advice-withdrawal')}
        canClose={targetMap.has('pre-advice-close')}
        onClose={() => setDetailId(null)}
        onAction={(id, prefill) => {
          setDetailId(null);
          openAction(id, prefill);
        }}
      />
      <ActionDrawer
        moduleId="warehouse-pre-advices"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => setTick((n) => n + 1)}
      />
    </>
  );
}
