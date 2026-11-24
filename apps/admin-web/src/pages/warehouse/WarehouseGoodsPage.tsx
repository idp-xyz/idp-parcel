import { useEffect, useState } from 'react';
import { Plus, RefreshCw } from 'lucide-react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { listOwnerGoods, type ApiResult, type GoodsListBody, type GoodsRecord } from './api';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';

const info = moduleInfoById['warehouse-goods'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const columns: ListColumn<GoodsRecord>[] = [
  { id: 'code', header: '商品编码', className: mono, render: (row) => row.goodsCode },
  { id: 'name', header: '名称', render: (row) => row.name },
  { id: 'unit', header: '基本单位', render: (row) => row.baseUnit },
  { id: 'track', header: '批次 / 效期', render: (row) => `${row.trackBatch ? '记批' : '不记批'} / ${row.trackExpiry ? '记效期' : '不记效期'}` },
  { id: 'composite', header: '组合商品', render: (row) => (row.composite ? '是' : '否') },
  { id: 'rev', header: '修订', className: mono, render: (row) => `r${row.revision}` },
  { id: 'at', header: '登记时刻', className: mono, render: (row) => formatInstant(row.registeredAt) },
];

/** 货主与商品页：先选货主（支持从客户账户下拉选），展示商品当前修订列表，弹窗代录与新修订。 */
export function WarehouseGoodsPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(receivingActionSpecs);
  const goodsTarget = targetMap.get('goods-on-behalf');

  const [owner, setOwner] = useState('');
  const [keyword, setKeyword] = useState('');
  const [answer, setAnswer] = useState<ApiResult<GoodsListBody> | null>(null);
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
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
    void listOwnerGoods(trimmed, keyword).then((res) => {
      if (!cancelled) setAnswer(res);
    });
    return () => {
      cancelled = true;
    };
  }, [owner, keyword, tick]);

  const openGoodsForm = (prefill?: GuidedFormValues) => {
    if (!goodsTarget) return;
    setDrawer((current) => ({
      target: goodsTarget,
      prefill: { ownerAccount: owner.trim(), ...prefill },
      nonce: (current?.nonce ?? 0) + 1,
    }));
  };

  const rows = answer?.kind === 'outcome' ? answer.body.goods : [];

  const inspectorOf = (row: GoodsRecord): InspectorContent => {
    const actions: InspectorAction[] = goodsTarget
      ? [
          {
            label: `代录新修订（基于 r${row.revision}）`,
            onRun: () =>
              openGoodsForm({
                ownerAccount: row.ownerAccount,
                goodsCode: row.goodsCode,
                basisRevision: String(row.revision),
                name: row.name,
                baseUnit: row.baseUnit,
                trackBatch: row.trackBatch,
                trackExpiry: row.trackExpiry,
              }),
          },
        ]
      : [];
    return {
      title: row.name,
      subtitle: row.goodsCode,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主', value: row.ownerAccount, mono: true },
            { label: '基本单位', value: row.baseUnit },
            { label: '批号管理', value: row.trackBatch ? '强制跟踪' : '不跟踪' },
            { label: '效期管理', value: row.trackExpiry ? '强制跟踪' : '不跟踪' },
            { label: '组合商品', value: row.composite ? '是' : '否' },
            { label: '当前修订', value: `r${row.revision}`, mono: true },
          ],
        },
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
        {
          kind: 'audit',
          fields: [
            { label: '登记人', value: row.registrant, mono: true },
            { label: '登记时刻', value: formatInstant(row.registeredAt), mono: true },
          ],
        },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<GoodsRecord>
        moduleId="warehouse-goods"
        title={info.title}
        description="按货主查阅商品档案当前修订。货主也可经客户接口自助维护；运营代录须带依据引用。"
        headerActions={
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" onClick={() => setTick((n) => n + 1)}>
              <RefreshCw className="h-3.5 w-3.5 mr-1" />
              刷新
            </Button>
            {goodsTarget ? (
              <Button type="button" className={primaryActionClass} onClick={() => openGoodsForm({ basisRevision: '0' })}>
                <Plus className="h-3.5 w-3.5 mr-1" />
                代录新商品
              </Button>
            ) : null}
          </div>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜商品编码或名称' }}
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
        rowKey={(row) => `${row.ownerAccount}/${row.goodsCode}`}
        inspector={inspectorOf}
        emptyRowsNote="该货主名下没有匹配的商品"
        viewState={
          owner.trim() === ''
            ? {
                kind: 'empty',
                title: '请先选择货主客户账户',
                description: '商品按货主隔离管理。在上方筛选栏选择或输入一个货主账户即可查阅其全部商品。',
              }
            : catalogueViewState(answer, rows.length, () => setTick((n) => n + 1), {
                module: info,
                endpoint: 'GET /warehouse-fulfillment-goods',
                emptyTitle: '该货主还没有商品档案',
                emptyDescription: '点右上角「代录新商品」为该货主建立首个商品修订。',
                narrowed: keyword.trim() !== '',
              })
        }
      />
      <ActionDrawer
        moduleId="warehouse-goods"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => setTick((n) => n + 1)}
      />
    </>
  );
}
