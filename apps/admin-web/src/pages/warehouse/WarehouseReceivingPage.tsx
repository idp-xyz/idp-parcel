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
  listDisposalRules,
  listOwnerGoods,
  listOwnerPreAdvices,
  listUnclaimedArrivals,
  type ApiResult,
  type DisposalRuleRecord,
  type GoodsListBody,
  type PreAdviceListBody,
  type UnclaimedArrivalRecord,
} from './api';
import { receivingActionSpecs } from './actions';
import { WarehouseActionPanel, useWarehouseActionTargets } from './ActionPanel';
import { problemNote } from './presentation';

const info = moduleInfoById['warehouse-receiving'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

type ArrivalStatusFilter = 'ALL' | 'UNCLAIMED' | 'CLAIMED' | 'DISPOSED';

const arrivalStatusLabels: Record<string, string> = {
  UNCLAIMED: '待认领',
  CLAIMED: '已认领',
  DISPOSED: '已处置',
};

const arrivalKindLabels: Record<string, string> = {
  INBOUND: '入库到货',
  RETURN: '退件',
};

const preAdviceStatusLabels: Record<string, string> = {
  ADVISED: '已预报',
  RECEIVING: '收货中',
  CLOSED: '已完结',
  CANCELLED: '已取消',
};

const arrivalColumns: ListColumn<UnclaimedArrivalRecord>[] = [
  { id: 'id', header: '到货号', className: mono, render: (row) => row.arrivalId },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'kind', header: '种类', render: (row) => arrivalKindLabels[row.kind] ?? row.kind },
  { id: 'status', header: '状态', render: (row) => arrivalStatusLabels[row.status] ?? row.status },
  { id: 'received', header: '登记时刻', className: mono, render: (row) => formatInstant(row.receivedAt) },
];

function arrivalInspector(row: UnclaimedArrivalRecord): InspectorContent {
  const fields = [
    { label: '事实标识', value: row.factId, mono: true },
    { label: '来源', value: row.origin },
    { label: '仓', value: row.warehouseCode, mono: true },
    { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
    { label: '观察', value: row.observation || '—' },
    { label: '货主线索', value: row.ownerClue || '—', mono: true },
    { label: '预报线索', value: row.preAdviceClue || '—', mono: true },
    { label: '存放处', value: row.storageLocation || '—', mono: true },
  ];
  if (row.disposition) {
    fields.push(
      { label: '处置方式', value: row.disposition.method },
      { label: '处置规则', value: `${row.disposition.ruleWarehouseCode} r${row.disposition.ruleRevision}`, mono: true },
      { label: '处置时刻', value: formatInstant(row.disposition.disposedAt), mono: true },
    );
  }
  return {
    title: row.arrivalId,
    subtitle: arrivalStatusLabels[row.status] ?? row.status,
    sections: [
      { kind: 'summary', fields },
      { kind: 'status', items: [{ label: '状态', word: arrivalStatusLabels[row.status] ?? row.status }] },
      { kind: 'audit', fields: [{ label: '操作者', value: row.operator, mono: true }, { label: '设备', value: row.device, mono: true }] },
    ],
  };
}

const ruleColumns: ListColumn<DisposalRuleRecord>[] = [
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'revision', header: '修订', className: mono, render: (row) => String(row.revision) },
  { id: 'window', header: '认领期限（秒）', className: mono, render: (row) => String(row.claimWindowSeconds) },
  { id: 'methods', header: '可用处置方式', render: (row) => row.disposalMethods.join('、') },
  { id: 'at', header: '登记时刻', className: mono, render: (row) => formatInstant(row.registeredAt) },
];

function ruleInspector(row: DisposalRuleRecord): InspectorContent {
  return {
    title: row.warehouseCode,
    subtitle: `第 ${row.revision} 版`,
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '认领期限（秒）', value: String(row.claimWindowSeconds), mono: true },
          { label: '处置方式', value: row.disposalMethods.join('、') },
          { label: '登记时刻', value: formatInstant(row.registeredAt), mono: true },
        ],
      },
      { kind: 'audit', fields: [{ label: '登记人', value: row.registrant, mono: true }] },
    ],
  };
}

function UnclaimedTable() {
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<ArrivalStatusFilter>('UNCLAIMED');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = {
    q: settled,
    filters: status === 'ALL' ? undefined : { status: [status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listUnclaimedArrivals);
  const rows = answer?.kind === 'outcome' ? answer.body.arrivals : [];

  return (
    <ListPageTemplate<UnclaimedArrivalRecord>
      moduleId="warehouse-receiving"
      title={info.title}
      description="待认领到货的运营查阅。收货确认、整箱收货和一线待认领登记由作业端发起；认领、撤销、处置和预报代录在「处理」签。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜到货号、仓或观察' }}
      filters={
        <>
          {(['ALL', 'UNCLAIMED', 'CLAIMED', 'DISPOSED'] as const).map((item) => (
            <button
              key={item}
              type="button"
              className={chipClass(item === status)}
              aria-pressed={item === status}
              onClick={() => setStatus(item)}
            >
              {item === 'ALL' ? '全部' : arrivalStatusLabels[item]}
            </button>
          ))}
        </>
      }
      columns={arrivalColumns}
      rows={rows}
      rowKey={(row) => row.arrivalId}
      inspector={arrivalInspector}
      pagination={pagination}
      emptyRowsNote="当前条件下没有匹配的待认领到货"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-unclaimed-arrivals',
        emptyTitle: status === 'UNCLAIMED' ? '没有待认领到货' : '还没有待认领记录',
        emptyDescription: '一线直接登记的待认领到货，以及预报取消或完结后到达的收货，会出现在这里。',
        narrowed: settled.trim() !== '' || (status !== 'UNCLAIMED' && status !== 'ALL'),
      })}
    />
  );
}

function DisposalRulesTable() {
  const [keyword, setKeyword] = useState('');
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = { q: settled };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listDisposalRules);
  const rows = answer?.kind === 'outcome' ? answer.body.disposalRules : [];

  return (
    <ListPageTemplate<DisposalRuleRecord>
      moduleId="warehouse-receiving"
      title="待认领处置规则"
      description="各仓当前生效的待认领处置规则修订。仓没有登记处置规则时，处置答未配置，不借用别仓。"
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜仓编码' }}
      columns={ruleColumns}
      rows={rows}
      rowKey={(row) => row.warehouseCode}
      inspector={ruleInspector}
      pagination={pagination}
      emptyRowsNote="当前条件下没有匹配的处置规则"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-unclaimed-disposal-rules',
        emptyTitle: '还没有登记处置规则',
        emptyDescription: '在「处理」签登记各仓的认领期限与处置方式。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

function OwnerPreAdviceLookup() {
  const [owner, setOwner] = useState('');
  const [keyword, setKeyword] = useState('');
  const [mode, setMode] = useState<'pre-advice' | 'goods'>('pre-advice');
  const [loading, setLoading] = useState(false);
  const [preAdvices, setPreAdvices] = useState<ApiResult<PreAdviceListBody> | null>(null);
  const [goods, setGoods] = useState<ApiResult<GoodsListBody> | null>(null);

  const query = () => {
    const trimmed = owner.trim();
    if (!trimmed) return;
    setLoading(true);
    if (mode === 'pre-advice') {
      void listOwnerPreAdvices(trimmed, keyword).then((result) => {
        setPreAdvices(result);
        setLoading(false);
      });
    } else {
      void listOwnerGoods(trimmed, keyword).then((result) => {
        setGoods(result);
        setLoading(false);
      });
    }
  };

  const activeResult = mode === 'pre-advice' ? preAdvices : goods;

  return (
    <div className="flex-1 overflow-auto p-4 space-y-4">
      <div className="flex flex-wrap items-end gap-2">
        <div className="flex gap-2">
          <button type="button" className={chipClass(mode === 'pre-advice')} onClick={() => setMode('pre-advice')}>
            入库预报
          </button>
          <button type="button" className={chipClass(mode === 'goods')} onClick={() => setMode('goods')}>
            货主商品
          </button>
        </div>
        <Input
          className="w-60"
          value={owner}
          onChange={(event) => setOwner(event.target.value)}
          placeholder="货主客户账户（必填）"
        />
        <Input
          className="w-60"
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          placeholder={mode === 'pre-advice' ? '预报号或仓编码（可选）' : '商品编码或名称（可选）'}
        />
        <Button type="button" disabled={loading || owner.trim() === ''} onClick={query}>
          {loading ? '查询中…' : '查询'}
        </Button>
      </div>

      {activeResult === null ? (
        <p className="type-body text-idpxyz-textMuted">输入货主客户账户后查询该货主的入库预报或商品当前修订。</p>
      ) : activeResult.kind !== 'outcome' ? (
        <p className="type-body text-idpxyz-danger">
          {activeResult.kind === 'callerProblem'
            ? `${problemNote(activeResult.code)}${activeResult.detail ? ` ${activeResult.detail}` : ''}`
            : activeResult.kind === 'noAnswer'
              ? problemNote(activeResult.code)
              : activeResult.kind === 'transport'
                ? activeResult.message
                : '当前登录身份无权查阅或接入渠道未配置。'}
        </p>
      ) : mode === 'pre-advice' && preAdvices?.kind === 'outcome' ? (
        preAdvices.body.preAdvices.length === 0 ? (
          <p className="type-body text-idpxyz-textMuted">该货主名下没有匹配的入库预报。</p>
        ) : (
          <div className="overflow-auto border border-idpxyz-border rounded">
            <table className="w-full text-left type-body">
              <thead className="border-b border-idpxyz-border text-idpxyz-textMuted">
                <tr>
                  <th className="px-3 py-2">预报号</th>
                  <th className="px-3 py-2">单据号</th>
                  <th className="px-3 py-2">仓</th>
                  <th className="px-3 py-2">入库方式</th>
                  <th className="px-3 py-2">状态</th>
                  <th className="px-3 py-2">受理时刻</th>
                </tr>
              </thead>
              <tbody>
                {preAdvices.body.preAdvices.map((item) => (
                  <tr key={item.preAdviceId} className="border-b border-idpxyz-border last:border-b-0">
                    <td className="px-3 py-2 font-mono">{item.preAdviceId}</td>
                    <td className="px-3 py-2 font-mono">{item.documentNumber ?? item.numberingAbsence ?? '—'}</td>
                    <td className="px-3 py-2 font-mono">{item.warehouseCode}</td>
                    <td className="px-3 py-2">{item.inboundMode}</td>
                    <td className="px-3 py-2">{preAdviceStatusLabels[item.status] ?? item.status}</td>
                    <td className="px-3 py-2 font-mono">{formatInstant(item.acceptedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      ) : goods?.kind === 'outcome' ? (
        goods.body.goods.length === 0 ? (
          <p className="type-body text-idpxyz-textMuted">该货主名下没有匹配的商品。</p>
        ) : (
          <div className="overflow-auto border border-idpxyz-border rounded">
            <table className="w-full text-left type-body">
              <thead className="border-b border-idpxyz-border text-idpxyz-textMuted">
                <tr>
                  <th className="px-3 py-2">商品编码</th>
                  <th className="px-3 py-2">名称</th>
                  <th className="px-3 py-2">基本单位</th>
                  <th className="px-3 py-2">批号 / 效期</th>
                  <th className="px-3 py-2">修订</th>
                  <th className="px-3 py-2">登记时刻</th>
                </tr>
              </thead>
              <tbody>
                {goods.body.goods.map((item) => (
                  <tr key={item.goodsCode} className="border-b border-idpxyz-border last:border-b-0">
                    <td className="px-3 py-2 font-mono">{item.goodsCode}</td>
                    <td className="px-3 py-2">{item.name}</td>
                    <td className="px-3 py-2">{item.baseUnit}</td>
                    <td className="px-3 py-2">{`${item.trackBatch ? '记批' : '不记批'} / ${item.trackExpiry ? '记效期' : '不记效期'}`}</td>
                    <td className="px-3 py-2 font-mono">{item.revision}</td>
                    <td className="px-3 py-2 font-mono">{formatInstant(item.registeredAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      ) : null}
    </div>
  );
}

/** 收货与待认领页：待认领到货列表、处置规则、按货主查预报与商品，以及相关代录/认领/处置动作。 */
export function WarehouseReceivingPage() {
  const targets = useWarehouseActionTargets(receivingActionSpecs);

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="unclaimed" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="unclaimed">待认领到货</TabsTrigger>
          <TabsTrigger value="rules">处置规则</TabsTrigger>
          <TabsTrigger value="owner-lookup">预报与商品查阅</TabsTrigger>
          {targets.length > 0 ? <TabsTrigger value="act">处理</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="unclaimed" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <UnclaimedTable />
        </TabsContent>
        <TabsContent value="rules" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DisposalRulesTable />
        </TabsContent>
        <TabsContent value="owner-lookup" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <OwnerPreAdviceLookup />
        </TabsContent>
        {targets.length > 0 ? (
          <TabsContent value="act" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <WarehouseActionPanel moduleId="warehouse-receiving" targets={targets} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
