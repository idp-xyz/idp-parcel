import { useEffect, useState } from 'react';
import { Plus, RefreshCw, TabletSmartphone } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useSettledKeyword } from '../use-catalogue-cursor';
import { getReturnsOverview, type ReturnPreAdviceRecord, type ReturnReceiptRecord, type ReturnsOverviewBody } from './api';
import type { ApiResult } from '../catalogue-api';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';
import { labelOf, WmsBadge, WmsDetailModal, WmsInfoGrid, WmsSubTable } from './wms-ui';

const info = moduleInfoById['warehouse-returns'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

function includes(value: string | undefined, keyword: string): boolean {
  if (keyword.trim() === '') return true;
  return (value ?? '').toLowerCase().includes(keyword.trim().toLowerCase());
}

/** 退货：预报台账与到仓处置台账分开。到仓扫描仍去作业台，回库处置在到仓行上直接做。 */
export function WarehouseReturnsPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(receivingActionSpecs);
  const [tab, setTab] = useState('advices');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [keyword, setKeyword] = useState('');
  const settled = useSettledKeyword(keyword);
  const [tick, setTick] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<ReturnsOverviewBody> | null>(null);
  const [advice, setAdvice] = useState<ReturnPreAdviceRecord | null>(null);
  const [receipt, setReceipt] = useState<ReturnReceiptRecord | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void getReturnsOverview().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [tick]);

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setAdvice(null);
    setReceipt(null);
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const advices = (body?.preAdvices ?? []).filter(
    (row) =>
      includes(row.adviceId, settled) ||
      includes(row.documentNumber, settled) ||
      includes(row.trackingNumber, settled) ||
      includes(row.ownerAccount, settled) ||
      includes(row.warehouseCode, settled),
  );
  const receipts = (body?.receipts ?? []).filter(
    (row) =>
      includes(row.factId, settled) ||
      includes(row.trackingNumber, settled) ||
      includes(row.ownerAccount, settled) ||
      includes(row.goodsCode, settled) ||
      includes(row.warehouseCode, settled),
  );

  const adviceColumns: ListColumn<ReturnPreAdviceRecord>[] = [
    { id: 'id', header: '预报号', className: mono, render: (row) => row.documentNumber ?? row.adviceId },
    { id: 'track', header: '退件单号', className: mono, render: (row) => row.trackingNumber },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'lines', header: '预计行', className: mono, render: (row) => String(row.lines?.length ?? 0) },
    { id: 'origin', header: '来源', render: (row) => labelOf(row.origin) },
    { id: 'at', header: '受理时刻', className: mono, render: (row) => formatInstant(row.receivedAt) },
  ];

  const receiptColumns: ListColumn<ReturnReceiptRecord>[] = [
    { id: 'fact', header: '到仓事实', className: mono, render: (row) => row.factId },
    { id: 'track', header: '退件单号', className: mono, render: (row) => row.trackingNumber },
    { id: 'goods', header: '商品', className: mono, render: (row) => row.goodsCode },
    { id: 'qty', header: '数量', className: mono, render: (row) => `${row.quantity} ${row.unit}` },
    { id: 'state', header: '入库状态', render: (row) => <WmsBadge code={row.stockState} /> },
    {
      id: 'disp',
      header: '回库处置',
      render: (row) => (row.disposition ? <WmsBadge code={row.disposition.destination} /> : <WmsBadge code="PENDING" label="待处置" />),
    },
    { id: 'at', header: '到仓时刻', className: mono, render: (row) => formatInstant(row.occurredAt) },
  ];

  const adviceInspector = (row: ReturnPreAdviceRecord): InspectorContent => ({
    title: row.documentNumber ?? row.adviceId,
    subtitle: row.trackingNumber,
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '货主 / 仓', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
          { label: '预计行数', value: String(row.lines?.length ?? 0), mono: true },
          { label: '来源', value: labelOf(row.origin) },
          { label: '依据', value: row.basis || '—', mono: true },
        ],
      },
      { kind: 'actions', actions: [{ label: '查看预报明细', onRun: () => setAdvice(row) }] },
    ],
  });

  const receiptInspector = (row: ReturnReceiptRecord): InspectorContent => {
    const actions: InspectorAction[] = [
      { label: '查看到仓明细', onRun: () => setReceipt(row) },
    ];
    if (!row.disposition && targetMap.has('return-disposition')) {
      actions.push({ label: '回库处置', onRun: () => openAction('return-disposition', { factId: row.factId }) });
    }
    return {
      title: row.trackingNumber || row.factId,
      subtitle: row.goodsCode,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '仓 / 库位', value: `${row.warehouseCode} · ${row.locationCode}`, mono: true },
            { label: '数量', value: `${row.quantity} ${row.unit}`, mono: true },
            { label: '货主', value: row.ownerAccount || '未认出', mono: true },
            { label: '处置', value: row.disposition ? labelOf(row.disposition.destination) : '待处置' },
          ],
        },
        { kind: 'actions', actions },
      ],
    };
  };

  const viewOf = (count: number) =>
    catalogueViewState(answer, count, () => setTick((n) => n + 1), {
      module: info,
      endpoint: 'GET /warehouse-fulfillment-returns-overview',
      emptyTitle: '还没有退货记录',
      emptyDescription: '代录退货预报，或在作业台扫描退件到仓后，记录会出现在这里。',
      narrowed: settled.trim() !== '',
    });

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="advices">退货预报</TabsTrigger>
          <TabsTrigger value="receipts">到仓与回库处置</TabsTrigger>
        </TabsList>

        <TabsContent value="advices" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<ReturnPreAdviceRecord>
            moduleId="warehouse-returns"
            title="退货预报"
            description="货主或运营事先登记的退回明细。预计商品不入库存账。双击看每一行。"
            headerActions={
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="outline" onClick={() => setTick((n) => n + 1)}>
                  <RefreshCw className="h-3.5 w-3.5 mr-1" />
                  刷新
                </Button>
                {targetMap.has('return-pre-advice') ? (
                  <Button type="button" className={primaryActionClass} onClick={() => openAction('return-pre-advice')}>
                    <Plus className="h-3.5 w-3.5 mr-1" />
                    代录退货预报
                  </Button>
                ) : null}
              </div>
            }
            search={{ value: keyword, onChange: setKeyword, placeholder: '搜预报号、退件单号、货主或仓' }}
            columns={adviceColumns}
            rows={advices}
            rowKey={(row) => row.adviceId}
            inspector={adviceInspector}
            onRowOpen={setAdvice}
            emptyRowsNote="当前条件下没有退货预报"
            viewState={viewOf(advices.length)}
          />
        </TabsContent>

        <TabsContent value="receipts" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<ReturnReceiptRecord>
            moduleId="warehouse-returns"
            title="到仓与回库处置"
            description="退件扫进暂存位后出现在这里。还没处置的行可以直接做良品上架、转次品或退还货主。"
            headerActions={
              <div className="flex flex-wrap gap-2">
                <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
                  <TabletSmartphone className="h-3.5 w-3.5 mr-1" />
                  去作业台扫描到仓
                </Button>
                {targetMap.has('return-disposition') ? (
                  <Button type="button" className={primaryActionClass} onClick={() => openAction('return-disposition')}>
                    代录回库处置
                  </Button>
                ) : null}
              </div>
            }
            search={{ value: keyword, onChange: setKeyword, placeholder: '搜事实号、退件单号、商品或货主' }}
            columns={receiptColumns}
            rows={receipts}
            rowKey={(row) => row.factId}
            inspector={receiptInspector}
            onRowOpen={setReceipt}
            emptyRowsNote="当前条件下没有退件到仓记录"
            viewState={viewOf(receipts.length)}
          />
        </TabsContent>
      </Tabs>

      <WmsDetailModal
        open={advice !== null}
        title={advice?.documentNumber ?? advice?.adviceId ?? ''}
        subtitle={advice?.trackingNumber}
        badge={advice ? <WmsBadge label={labelOf(advice.origin)} tone="info" /> : undefined}
        onClose={() => setAdvice(null)}
      >
        {advice ? (
          <>
            <WmsInfoGrid
              items={[
                { label: '预报号', value: advice.adviceId, mono: true },
                { label: '货主', value: advice.ownerAccount, mono: true },
                { label: '仓库', value: advice.warehouseCode, mono: true },
                { label: '退件单号', value: advice.trackingNumber, mono: true },
                { label: '依据', value: advice.basis || '—', mono: true },
                { label: '登记人', value: advice.operator || '—', mono: true },
                { label: '受理时刻', value: formatInstant(advice.receivedAt), mono: true },
                { label: '单号状态', value: advice.documentNumber ?? advice.numberingAbsence ?? '—', mono: true },
              ]}
            />
            <WmsSubTable
              title="预计退回"
              count={advice.lines?.length ?? 0}
              emptyText="这张预报没有商品行。"
              rows={advice.lines ?? []}
              rowKey={(line, index) => `${line.goodsCode}-${index}`}
              columns={[
                { id: 'goods', header: '商品', mono: true, render: (line) => line.goodsCode },
                { id: 'unit', header: '单位', render: (line) => line.unit },
                { id: 'qty', header: '数量', align: 'right', mono: true, render: (line) => String(line.quantity) },
              ]}
            />
          </>
        ) : null}
      </WmsDetailModal>

      <WmsDetailModal
        open={receipt !== null}
        title={receipt?.trackingNumber || receipt?.factId || ''}
        subtitle={receipt?.factId}
        badge={
          receipt ? (
            receipt.disposition ? <WmsBadge code={receipt.disposition.destination} /> : <WmsBadge code="PENDING" label="待处置" />
          ) : undefined
        }
        actions={
          receipt && !receipt.disposition && targetMap.has('return-disposition') ? (
            <Button type="button" className={primaryActionClass} onClick={() => openAction('return-disposition', { factId: receipt.factId })}>
              回库处置
            </Button>
          ) : undefined
        }
        onClose={() => setReceipt(null)}
      >
        {receipt ? (
          <WmsInfoGrid
            items={[
              { label: '仓库', value: receipt.warehouseCode, mono: true },
              { label: '暂存库位', value: receipt.locationCode, mono: true },
              { label: '货主', value: receipt.ownerAccount || '未认出', mono: true },
              { label: '关联预报', value: receipt.adviceId || '—', mono: true },
              { label: '待认领号', value: receipt.arrivalId || '—', mono: true },
              { label: '商品', value: receipt.goodsCode, mono: true },
              { label: '数量', value: `${receipt.quantity} ${receipt.unit}`, mono: true },
              { label: '入库状态', value: labelOf(receipt.stockState) },
              { label: '到仓结果', value: labelOf(receipt.outcome) },
              { label: '到仓时刻', value: formatInstant(receipt.occurredAt), mono: true },
              { label: '处置去向', value: receipt.disposition ? labelOf(receipt.disposition.destination) : '待处置' },
              { label: '处置时刻', value: receipt.disposition ? formatInstant(receipt.disposition.decidedAt) : '—', mono: true },
            ]}
          />
        ) : null}
      </WmsDetailModal>

      <ActionDrawer
        moduleId="warehouse-returns"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => setTick((n) => n + 1)}
      />
    </div>
  );
}
