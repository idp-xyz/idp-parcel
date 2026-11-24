import { useEffect, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { Button, Input, Tabs, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { formatInstant } from '../catalogue-view';
import { queryOwnerStock, type ApiResult, type StockReadBody, type StockViewKind } from './api';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { problemNote } from './presentation';
import { useWarehouseLookups } from './use-catalogues';

const info = moduleInfoById['warehouse-stock'];

const stockStateLabels: Record<string, string> = {
  GOOD: '良品',
  DEFECTIVE: '次品',
};

const holdKindLabels: Record<string, string> = {
  FROZEN: '手动冻结',
  RESERVED: '出库预占',
  LOCKED: '作业锁',
  CARTON_FROZEN: '整箱冻结',
};

const thClass = 'px-3 py-2 text-left type-caption font-medium text-idpxyz-textMuted border-b border-idpxyz-border whitespace-nowrap';
const tdClass = 'px-3 py-2 type-body text-idpxyz-text border-b border-idpxyz-border/60 align-top';
const monoTd = `${tdClass} font-mono whitespace-nowrap`;

/** 实时库存查询页：全部五视图以结构化数据表展示（零原始 JSON），每条库存格/整箱直接带出冻结、次品转良品与库存调整弹窗。 */
export function WarehouseStockPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(internalOpsActionSpecs);

  const [owner, setOwner] = useState('');
  const [warehouse, setWarehouse] = useState('');
  const [view, setView] = useState<StockViewKind>('cells');
  const [answer, setAnswer] = useState<ApiResult<StockReadBody> | null>(null);
  const [loading, setLoading] = useState(false);
  const [tick, setTick] = useState(0);
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);

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
    setLoading(true);
    void queryOwnerStock(trimmed, warehouse, view).then((res) => {
      if (!cancelled) {
        setAnswer(res);
        setLoading(false);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [owner, warehouse, view, tick]);

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const body = answer?.kind === 'outcome' ? answer.body : null;

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <div className="px-6 pt-5 pb-3 border-b border-idpxyz-border bg-idpxyz-panel space-y-3 shrink-0">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h1 className="type-title font-bold text-idpxyz-textBright">{info.title}</h1>
            <p className="mt-1 type-body text-idpxyz-textMuted">
              按货主与仓库查阅实时账面库存，直接从商品格或整箱发起冻结、解冻、次品转良品或库存调整。
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {targetMap.has('stock-unfreeze') ? (
              <Button type="button" variant="outline" onClick={() => openAction('stock-unfreeze')}>
                按冻结号解冻
              </Button>
            ) : null}
            <Button type="button" variant="outline" disabled={loading || owner.trim() === ''} onClick={() => setTick((n) => n + 1)}>
              <RefreshCw className="h-3.5 w-3.5 mr-1.5" />
              {loading ? '查询中…' : '刷新库存'}
            </Button>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-1.5">
            <span className="type-caption text-idpxyz-textMuted">货主：</span>
            {lookups.owners.length > 0 ? (
              <select
                className="h-8 rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text"
                value={lookups.owners.some((o) => o.accountId === owner) ? owner : ''}
                onChange={(event) => {
                  if (event.target.value) setOwner(event.target.value);
                }}
              >
                <option value="">选择货主…</option>
                {lookups.owners.map((acct) => (
                  <option key={acct.accountId} value={acct.accountId}>
                    {acct.customerPartyName ? `${acct.accountId} · ${acct.customerPartyName}` : acct.accountId}
                  </option>
                ))}
              </select>
            ) : null}
            <Input
              className="h-8 w-44 font-mono type-body"
              value={owner}
              onChange={(event) => setOwner(event.target.value)}
              placeholder="货主账户编码"
            />
          </div>

          <div className="flex items-center gap-1.5">
            <span className="type-caption text-idpxyz-textMuted">仓库：</span>
            <select
              className="h-8 rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text"
              value={warehouse}
              onChange={(event) => setWarehouse(event.target.value)}
            >
              <option value="">全部仓库</option>
              {lookups.warehouses.map((w) => (
                <option key={w.warehouseCode} value={w.warehouseCode}>
                  {`${w.warehouseCode} · ${w.name}`}
                </option>
              ))}
            </select>
          </div>
        </div>

        <Tabs value={view} onValueChange={(next) => setView(next as StockViewKind)}>
          <TabsList>
            <TabsTrigger value="cells">商品库存格</TabsTrigger>
            <TabsTrigger value="cartons">整箱库存</TabsTrigger>
            <TabsTrigger value="summary">库存汇总</TabsTrigger>
            <TabsTrigger value="holds">占用与冻结</TabsTrigger>
            <TabsTrigger value="movements">库存流水</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      <div className="flex-1 overflow-auto p-6">
        {owner.trim() === '' ? (
          <p className="type-body text-idpxyz-textMuted">请先在上方选择一个货主客户账户。</p>
        ) : answer && answer.kind !== 'outcome' ? (
          <div className="rounded border border-idpxyz-danger/40 bg-idpxyz-panel p-4 type-body text-idpxyz-danger">
            {answer.kind === 'callerProblem'
              ? answer.detail || problemNote(answer.code)
              : answer.kind === 'noAnswer'
                ? problemNote(answer.code)
                : answer.kind === 'transport'
                  ? answer.message
                  : answer.kind === 'notGranted'
                    ? '当前账号没有查询该货主库存的授予。'
                    : answer.kind === 'loginRequired'
                      ? '请先登录后再查询库存。'
                      : '该能力在当前租户尚未配置。'}
          </div>
        ) : loading && !body ? (
          <p className="type-body text-idpxyz-textMuted">正在查询实时库存…</p>
        ) : body ? (
          <div className="rounded-lg border border-idpxyz-border bg-idpxyz-panel overflow-x-auto">
            {view === 'cells' ? (
              (body.cells ?? []).length === 0 ? (
                <p className="p-6 type-body text-idpxyz-textMuted">当前条件下没有商品库存格。</p>
              ) : (
                <table className="w-full border-collapse">
                  <thead>
                    <tr className="bg-idpxyz-editor/60">
                      <th className={thClass}>仓库 / 库位</th>
                      <th className={thClass}>商品编码</th>
                      <th className={thClass}>单位 / 状态</th>
                      <th className={thClass}>批次 / 批号 / 效期</th>
                      <th className={thClass}>账面 / 冻结 / 可用</th>
                      <th className={thClass}>库龄</th>
                      <th className={thClass}>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(body.cells ?? []).map((cell, index) => (
                      <tr key={`${cell.warehouse}-${cell.location ?? ''}-${cell.goodsCode}-${cell.inboundBatchId}-${index}`} className="hover:bg-idpxyz-hover/40">
                        <td className={monoTd}>{`${cell.warehouse} / ${cell.location ?? '代理仓(无库位)'}`}</td>
                        <td className={monoTd}>
                          {cell.goodsCode}
                          {cell.labelAlias ? <span className="ml-1 type-caption text-idpxyz-textMuted">({cell.labelAlias})</span> : null}
                        </td>
                        <td className={tdClass}>{`${cell.unit} · ${stockStateLabels[cell.stockState] ?? cell.stockState}`}</td>
                        <td className={monoTd}>
                          {cell.inboundBatchId}
                          {cell.batchNumber ? ` · 批:${cell.batchNumber}` : ''}
                          {cell.expiryDate ? ` · 效:${cell.expiryDate}` : ''}
                        </td>
                        <td className={monoTd}>{`${cell.quantity} / ${cell.frozen} / ${cell.available}`}</td>
                        <td className={monoTd}>{cell.ageDays !== undefined ? `${cell.ageDays} 天` : cell.age ?? '未配置'}</td>
                        <td className={tdClass}>
                          <div className="flex flex-wrap gap-1.5">
                            {targetMap.has('stock-freeze') && cell.location ? (
                              <Button
                                type="button"
                                size="sm"
                                variant="outline"
                                onClick={() =>
                                  openAction('stock-freeze', {
                                    warehouse: cell.warehouse,
                                    ownerAccount: owner.trim(),
                                    locationCode: cell.location ?? '',
                                    goodsCode: cell.goodsCode,
                                    unit: cell.unit,
                                    stockState: cell.stockState,
                                    inboundBatchId: cell.inboundBatchId,
                                    quantity: String(Math.max(1, cell.available)),
                                  })
                                }
                              >
                                冻结
                              </Button>
                            ) : null}
                            {targetMap.has('condition-restore') && cell.stockState === 'DEFECTIVE' && cell.location ? (
                              <Button
                                type="button"
                                size="sm"
                                variant="outline"
                                onClick={() =>
                                  openAction('condition-restore', {
                                    warehouse: cell.warehouse,
                                    ownerAccount: owner.trim(),
                                    locationCode: cell.location ?? '',
                                    goodsCode: cell.goodsCode,
                                    unit: cell.unit,
                                    stockState: 'DEFECTIVE',
                                    inboundBatchId: cell.inboundBatchId,
                                    quantity: String(Math.max(1, cell.available)),
                                  })
                                }
                              >
                                次转良
                              </Button>
                            ) : null}
                            {targetMap.has('adjustment-submit') && cell.location ? (
                              <Button
                                type="button"
                                size="sm"
                                variant="outline"
                                onClick={() =>
                                  openAction('adjustment-submit', {
                                    warehouse: cell.warehouse,
                                    ownerAccount: owner.trim(),
                                    lines: [
                                      {
                                        kind: 'DECREASE',
                                        locationCode: cell.location ?? '',
                                        goodsCode: cell.goodsCode,
                                        unit: cell.unit,
                                        stockState: cell.stockState,
                                        quantity: '1',
                                        inboundBatchId: cell.inboundBatchId,
                                      },
                                    ],
                                  })
                                }
                              >
                                调账
                              </Button>
                            ) : null}
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
            ) : null}

            {view === 'cartons' ? (
              (body.cartons ?? []).length === 0 ? (
                <p className="p-6 type-body text-idpxyz-textMuted">当前条件下没有整箱库存。</p>
              ) : (
                <table className="w-full border-collapse">
                  <thead>
                    <tr className="bg-idpxyz-editor/60">
                      <th className={thClass}>仓库 / 库位</th>
                      <th className={thClass}>整箱箱号</th>
                      <th className={thClass}>箱内清单</th>
                      <th className={thClass}>冻结/占用</th>
                      <th className={thClass}>入库时刻 / 库龄</th>
                      <th className={thClass}>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(body.cartons ?? []).map((carton) => (
                      <tr key={`${carton.warehouse}-${carton.cartonNumber}`} className="hover:bg-idpxyz-hover/40">
                        <td className={monoTd}>{`${carton.warehouse} / ${carton.location ?? '—'}`}</td>
                        <td className={monoTd}>{carton.cartonNumber}</td>
                        <td className={tdClass}>
                          {carton.bill && carton.bill.length > 0
                            ? carton.bill.map((b) => `${b.goodsCode}×${b.quantity}`).join('，')
                            : '未报清单'}
                        </td>
                        <td className={tdClass}>{carton.held ? '已冻结/锁定' : '可用'}</td>
                        <td className={monoTd}>
                          {`${formatInstant(carton.inboundAt)} · ${carton.ageDays !== undefined ? `${carton.ageDays} 天` : carton.age ?? '未配置'}`}
                        </td>
                        <td className={tdClass}>
                          {targetMap.has('stock-freeze') && !carton.held ? (
                            <Button
                              type="button"
                              size="sm"
                              variant="outline"
                              onClick={() =>
                                openAction('stock-freeze', {
                                  warehouse: carton.warehouse,
                                  ownerAccount: owner.trim(),
                                  cartonNumber: carton.cartonNumber,
                                })
                              }
                            >
                              冻结整箱
                            </Button>
                          ) : null}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
            ) : null}

            {view === 'summary' ? (
              (body.summary ?? []).length === 0 ? (
                <p className="p-6 type-body text-idpxyz-textMuted">当前条件下没有汇总数据。</p>
              ) : (
                <table className="w-full border-collapse">
                  <thead>
                    <tr className="bg-idpxyz-editor/60">
                      <th className={thClass}>仓库</th>
                      <th className={thClass}>商品编码</th>
                      <th className={thClass}>单位 / 状态</th>
                      <th className={thClass}>在库</th>
                      <th className={thClass}>可分配</th>
                      <th className={thClass}>冻结</th>
                      <th className={thClass}>出库预占</th>
                      <th className={thClass}>作业锁</th>
                      <th className={thClass}>立即可用</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(body.summary ?? []).map((row, index) => (
                      <tr key={`${row.warehouse}-${row.goodsCode}-${row.unit}-${row.stockState}-${index}`} className="hover:bg-idpxyz-hover/40">
                        <td className={monoTd}>{row.warehouse}</td>
                        <td className={monoTd}>{row.goodsCode}</td>
                        <td className={tdClass}>{`${row.unit} · ${stockStateLabels[row.stockState] ?? row.stockState}`}</td>
                        <td className={monoTd}>{row.onHand}</td>
                        <td className={monoTd}>{row.allocatable}</td>
                        <td className={monoTd}>{row.frozen}</td>
                        <td className={monoTd}>{row.reserved}</td>
                        <td className={monoTd}>{row.locked}</td>
                        <td className={`${monoTd} font-semibold text-idpxyz-textBright`}>{row.available}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
            ) : null}

            {view === 'holds' ? (
              (body.holds ?? []).length === 0 ? (
                <p className="p-6 type-body text-idpxyz-textMuted">当前没有冻结或占用记录。</p>
              ) : (
                <table className="w-full border-collapse">
                  <thead>
                    <tr className="bg-idpxyz-editor/60">
                      <th className={thClass}>类型</th>
                      <th className={thClass}>仓库 / 库位</th>
                      <th className={thClass}>商品 / 整箱</th>
                      <th className={thClass}>数量</th>
                      <th className={thClass}>原因 / 操作者</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(body.holds ?? []).map((hold, index) => (
                      <tr key={`${hold.kind}-${hold.warehouse}-${hold.goodsCode ?? hold.cartonNumber ?? ''}-${index}`} className="hover:bg-idpxyz-hover/40">
                        <td className={tdClass}>{holdKindLabels[hold.kind] ?? hold.kind}</td>
                        <td className={monoTd}>{`${hold.warehouse} / ${hold.location ?? '—'}`}</td>
                        <td className={monoTd}>{hold.cartonNumber ? `整箱 ${hold.cartonNumber}` : `${hold.goodsCode ?? '—'} (${hold.unit ?? ''})`}</td>
                        <td className={monoTd}>{hold.quantity}</td>
                        <td className={tdClass}>{`${hold.reason ?? '—'}${hold.operator ? ` · ${hold.operator}` : ''}`}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
            ) : null}

            {view === 'movements' ? (
              (body.movements ?? []).length === 0 ? (
                <p className="p-6 type-body text-idpxyz-textMuted">当前条件下没有库存变动流水。</p>
              ) : (
                <table className="w-full border-collapse">
                  <thead>
                    <tr className="bg-idpxyz-editor/60">
                      <th className={thClass}>仓库</th>
                      <th className={thClass}>变动原因</th>
                      <th className={thClass}>来源事实 / 单据</th>
                      <th className={thClass}>明细变动</th>
                      <th className={thClass}>发生时刻</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(body.movements ?? []).map((mov, index) => (
                      <tr key={`${mov.warehouse}-${mov.source ?? ''}-${mov.occurredAt}-${index}`} className="hover:bg-idpxyz-hover/40">
                        <td className={monoTd}>{mov.warehouse}</td>
                        <td className={tdClass}>{mov.reason}</td>
                        <td className={monoTd}>{mov.source ?? '—'}</td>
                        <td className={tdClass}>
                          {(mov.lines ?? []).map((line, i) => (
                            <div key={i} className="font-mono type-caption">
                              {`${line.location ?? '仓'} · ${line.goodsCode}(${line.unit}/${stockStateLabels[line.stockState] ?? line.stockState}) ${line.delta > 0 ? `+${line.delta}` : line.delta}`}
                            </div>
                          ))}
                        </td>
                        <td className={monoTd}>{formatInstant(mov.occurredAt)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )
            ) : null}
          </div>
        ) : null}
      </div>

      <ActionDrawer
        moduleId="warehouse-stock"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => setTick((n) => n + 1)}
      />
    </div>
  );
}
