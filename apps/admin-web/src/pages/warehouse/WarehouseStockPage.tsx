import { useEffect, useMemo, useState, type ComponentProps, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button, Input, Tabs, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, type WorkbenchColumn } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { statusToneText } from '../../components/status-tone';
import { moduleInfoById } from '../../navigation';
import {
  currentCursorAfter,
  cursorPageNumber,
  cursorPageSummary,
  cursorTrailFor,
  nextCursorPage,
  previousCursorPage,
  startCursorTrail,
  type CursorTrail,
} from '../../templates/cursor-pagination';
import { catalogueDefaultPageSize, cataloguePageSizes } from '../catalogue-query';
import { useSettledKeyword } from '../use-catalogue-cursor';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { queryOwnerStock, type ApiResult, type StockReadBody, type StockViewKind } from './api';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { useWarehouseLookups, type WarehouseLookups } from './use-catalogues';
import { labelOf, WmsBadge, WmsFilterSelect, WmsPagerBar, WmsTwoLine } from './wms-ui';

const info = moduleInfoById['warehouse-stock'];

type Cell = NonNullable<StockReadBody['cells']>[number];
type Carton = NonNullable<StockReadBody['cartons']>[number];
type Summary = NonNullable<StockReadBody['summary']>[number];
type Hold = NonNullable<StockReadBody['holds']>[number];
type Movement = NonNullable<StockReadBody['movements']>[number];

const holdKindLabels: Record<string, string> = {
  FROZEN: '手动冻结',
  RESERVED: '出库预占',
  LOCKED: '作业锁',
  CARTON_FROZEN: '整箱冻结',
};

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

function allowedSize(value: number): number {
  return (cataloguePageSizes as readonly number[]).includes(value) ? value : catalogueDefaultPageSize;
}

function ageText(ageDays: number | undefined, age: string | undefined): string {
  if (ageDays !== undefined) return `${ageDays} 天`;
  return age ?? '未配置';
}

/**
 * 数量按能不能动上色，零保持淡色，免得一表都是颜色。
 * 在库是事实，不上色。冻着、被作业锁住用警示色；已经答应给出库的预占、还能分配的用提示色；
 * 可发有数用成功色，库里有货却一件都发不出用危险色。流水里增加是成功色，减少是危险色。
 */
function Qty({
  value,
  kind,
  onHand = 0,
}: {
  value: number;
  kind: 'onHand' | 'frozen' | 'reserved' | 'locked' | 'allocatable' | 'free' | 'delta';
  /** 可发为 0 时用来判断是空库位，还是有货发不出。 */
  onHand?: number;
}) {
  const tone =
    kind === 'delta'
      ? value > 0
        ? statusToneText.success
        : value < 0
          ? statusToneText.danger
          : 'text-idpxyz-textMuted'
      : value === 0
        ? kind === 'free' && onHand > 0
          ? statusToneText.danger
          : 'text-idpxyz-textMuted'
        : kind === 'onHand'
          ? 'text-idpxyz-text'
          : kind === 'frozen' || kind === 'locked'
            ? statusToneText.warning
            : kind === 'reserved' || kind === 'allocatable'
              ? statusToneText.info
              : statusToneText.success;
  const text = kind === 'delta' && value > 0 ? `+${value}` : String(value);
  return <span className={`whitespace-nowrap tabular-nums ${tone}`}>{text}</span>;
}

function named(names: ReadonlyMap<string, string>, code: string) {
  const name = nameOf(names, code);
  return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
}

/** 实时库存。五种看法共用一次查阅：要一位货主，仓可以不限。检索词按商品、条码、库位或箱号交给读口，先收窄再翻页。流水按游标翻，其余四种按页码向读口要。 */
export function WarehouseStockPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(internalOpsActionSpecs);
  const [storedOwner, setOwner] = useTabViewState('warehouse-stock.owner', '');
  const [warehouse, setWarehouse] = useTabViewState('warehouse-stock.warehouse', '');
  const [keyword, setKeyword] = useTabViewState('warehouse-stock.keyword', '');
  const [view, setView] = useTabViewState<StockViewKind>('warehouse-stock.view', 'cells');
  const [openByView, setOpenByView] = useTabViewState<Partial<Record<StockViewKind, string | null>>>('warehouse-stock.open', {});
  const [storedMovementSize, setMovementSize] = useTabViewState('warehouse-stock.movementSize', catalogueDefaultPageSize);
  const [storedSheetSize, setSheetSize] = useTabViewState('warehouse-stock.sheetSize', catalogueDefaultPageSize);
  const [slot, setSlot] = useState<{ scope: string; answer: ApiResult<StockReadBody> } | null>(null);
  const [loading, setLoading] = useState(false);
  const [tick, setTick] = useState(0);
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [paging, setPaging] = useState({ conditions: '', page: 1 });
  const [trail, setTrail] = useState<CursorTrail>(() => startCursorTrail(''));

  const owner = storedOwner;
  const settled = useSettledKeyword(keyword);
  const movementSize = allowedSize(storedMovementSize);
  const sheetSize = allowedSize(storedSheetSize);
  const openKey = openByView[view] ?? null;
  const setOpenKey = (key: string | null) => setOpenByView((current) => ({ ...current, [view]: key }));

  useEffect(() => {
    if (owner === '' && lookups.owners.length > 0) setOwner(lookups.owners[0].accountId);
  }, [lookups.owners, owner, setOwner]);

  // 页码与游标跟着查阅条件：货主、仓、看法、检索词或页大小一换就从第一页看起。流水按游标翻，其余四种按页码。
  const conditions = `${owner.trim()}|${warehouse}|${view}|${settled.trim()}|${view === 'movements' ? movementSize : sheetSize}`;
  if (trail.query !== conditions) {
    setTrail(startCursorTrail(conditions));
    setPaging({ conditions, page: 1 });
  }
  const cursor = cursorTrailFor(trail, conditions);
  const after = view === 'movements' ? currentCursorAfter(cursor) : null;
  const sheetPage = paging.conditions === conditions ? paging.page : 1;
  const scope = `${owner.trim()}|${warehouse}|${view}|${settled.trim()}`;

  useEffect(() => {
    const trimmed = owner.trim();
    if (!trimmed) return;
    let cancelled = false;
    setLoading(true);
    void queryOwnerStock(
      trimmed,
      warehouse,
      view,
      after,
      view === 'movements' ? movementSize : sheetSize,
      view === 'movements' ? undefined : sheetPage,
      settled,
    ).then((res) => {
      if (!cancelled) {
        setSlot({ scope, answer: res });
        setLoading(false);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [owner, warehouse, view, settled, tick, after, movementSize, sheetSize, sheetPage, scope]);

  const answer = slot && slot.scope === scope ? slot.answer : null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const movementPage = body?.page ?? null;
  const movementPageNumber = cursorPageNumber(cursor);
  const book = body?.book ?? null;
  const pager =
    view === 'movements'
      ? {
          summary: cursorPageSummary(movementPageNumber, movementPage?.size ?? movementSize, movementPage?.total ?? null),
          canPrevious: !loading && movementPageNumber > 1,
          canNext: !loading && movementPage !== null && movementPage.next !== null,
          onPrevious: () => setTrail(previousCursorPage(cursor)),
          onNext: () => setTrail(nextCursorPage(cursor, movementPage?.next ?? null)),
          pageSize: { value: movementSize, options: cataloguePageSizes, onChange: setMovementSize },
        }
      : {
          summary: cursorPageSummary(book?.page ?? sheetPage, book?.pageSize ?? sheetSize, book?.total ?? null),
          canPrevious: !loading && sheetPage > 1,
          canNext: !loading && book !== null && sheetPage * (book.pageSize || sheetSize) < book.total,
          onPrevious: () => setPaging({ conditions, page: sheetPage - 1 }),
          onNext: () => setPaging({ conditions, page: sheetPage + 1 }),
          pageSize: { value: sheetSize, options: cataloguePageSizes, onChange: setSheetSize },
        };

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };
  const can = (id: string) => targetMap.has(id);
  const warehouseNames = useMemo(
    () => new Map(lookups.warehouses.map((item) => [item.warehouseCode, item.name ?? ''])),
    [lookups.warehouses],
  );
  const ownerNames = useMemo(
    () => new Map(lookups.owners.map((account) => [account.accountId, account.customerPartyName ?? ''])),
    [lookups.owners],
  );
  const refresh = () => setTick((count) => count + 1);
  const chooseView = (next: string) => {
    setView(next as StockViewKind);
  };

  const narrowed = warehouse !== '' || settled.trim() !== '' || (view === 'movements' ? movementPageNumber > 1 : sheetPage > 1);
  const rowCount =
    view === 'cells'
      ? (body?.cells?.length ?? 0)
      : view === 'cartons'
        ? (body?.cartons?.length ?? 0)
        : view === 'summary'
          ? (body?.summary?.length ?? 0)
          : view === 'holds'
            ? (body?.holds?.length ?? 0)
            : (body?.movements?.length ?? 0);
  const emptyTitle =
    view === 'cells'
      ? '当前没有商品库存格'
      : view === 'cartons'
        ? '当前没有整箱库存'
        : view === 'summary'
          ? '当前没有汇总'
          : view === 'holds'
            ? '当前没有冻结或占用'
            : '当前没有库存流水';
  const viewState: TemplateViewState =
    owner.trim() === ''
      ? lookups.loaded
        ? { kind: 'empty', title: '还没有货主客户账户', description: '库存按货主记。先在国际小包那边的「客户」里登记货主的客户账户。' }
        : { kind: 'loading', shape: 'table' }
      : answer === null
        ? { kind: 'loading', shape: 'table' }
        : catalogueViewState(answer, rowCount, refresh, {
            module: info,
            endpoint: 'GET /warehouse-fulfillment-stock',
            emptyTitle,
            emptyDescription: '换个商品、库位或箱号，或换一个货主、仓库再查。流水只保留已经入账的变动。',
            narrowed,
          });

  const frame = {
    lookups,
    owner,
    setOwner,
    warehouse,
    setWarehouse,
    view,
    chooseView,
    ownerNames,
    warehouseNames,
    openKey,
    setOpenKey,
    viewState,
    refreshing: loading,
    onRefresh: refresh,
    pager: owner.trim() !== '' && (body !== null || (view === 'movements' && movementPageNumber > 1)) && (viewState.kind === 'ready' || view === 'movements') ? pager : undefined,
    can,
    openAction,
    keyword,
    setKeyword,
    unfreeze: can('stock-unfreeze') ? (
      <Button type="button" variant="outline" onClick={() => openAction('stock-unfreeze')}>
        按冻结号解冻
      </Button>
    ) : undefined,
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      {view === 'cells' ? <CellSheet frame={frame} cells={body?.cells ?? []} /> : null}
      {view === 'cartons' ? <CartonSheet frame={frame} cartons={body?.cartons ?? []} /> : null}
      {view === 'summary' ? <SummarySheet frame={frame} summary={body?.summary ?? []} /> : null}
      {view === 'holds' ? <HoldSheet frame={frame} holds={body?.holds ?? []} /> : null}
      {view === 'movements' ? <MovementSheet frame={frame} movements={body?.movements ?? []} /> : null}
      <ActionDrawer moduleId="warehouse-stock" active={drawer} lookups={lookups} onClose={() => setDrawer(null)} onSubmitted={refresh} />
    </div>
  );
}

interface Frame {
  lookups: WarehouseLookups;
  owner: string;
  setOwner: (value: string) => void;
  warehouse: string;
  setWarehouse: (value: string) => void;
  view: StockViewKind;
  chooseView: (next: string) => void;
  ownerNames: ReadonlyMap<string, string>;
  warehouseNames: ReadonlyMap<string, string>;
  openKey: string | null;
  setOpenKey: (key: string | null) => void;
  viewState: TemplateViewState;
  refreshing: boolean;
  onRefresh: () => void;
  pager: ComponentProps<typeof WmsPagerBar> | undefined;
  can: (id: string) => boolean;
  openAction: (id: string, prefill?: GuidedFormValues) => void;
  keyword: string;
  setKeyword: (value: string) => void;
  unfreeze: ReactNode;
}

function StockFrame<Row>({
  frame,
  columns,
  rows,
  rowKey,
  noMatch,
  renderDetail,
}: {
  frame: Frame;
  columns: WorkbenchColumn<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
  noMatch: string;
  renderDetail: (key: string) => ReactNode;
}) {
  const ownerName = nameOf(frame.ownerNames, frame.owner.trim());
  return (
    <WorkbenchPageTemplate<Row>
      title={info.title}
      viewStateKey="warehouse-stock"
      subtitle={frame.owner.trim() === '' ? '先选定一位货主' : `正在看${ownerName || frame.owner.trim()}的账面`}
      titleAside={
        <Tabs value={frame.view} onValueChange={frame.chooseView}>
          <TabsList>
            <TabsTrigger value="cells">商品库存格</TabsTrigger>
            <TabsTrigger value="cartons">整箱库存</TabsTrigger>
            <TabsTrigger value="summary">库存汇总</TabsTrigger>
            <TabsTrigger value="holds">占用与冻结</TabsTrigger>
            <TabsTrigger value="movements">库存流水</TabsTrigger>
          </TabsList>
        </Tabs>
      }
      onRefresh={frame.onRefresh}
      refreshing={frame.refreshing}
      primaryAction={frame.unfreeze}
      search={{ value: frame.keyword, onChange: frame.setKeyword, placeholder: '搜商品、条码、库位或箱号' }}
      extraFilters={
        <>
          <label className="inline-flex items-center gap-1.5 type-body text-idpxyz-textMuted">
            <span className="whitespace-nowrap">货主</span>
            <select
              className="h-7 max-w-[16rem] rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text focus:border-idpxyz-accent focus:outline-none"
              value={frame.lookups.owners.some((account) => account.accountId === frame.owner) ? frame.owner : ''}
              onChange={(event) => {
                if (event.target.value) frame.setOwner(event.target.value);
              }}
            >
              {frame.lookups.owners.length === 0 ? <option value="">没有货主客户账户</option> : null}
              {frame.lookups.owners.map((account) => (
                <option key={account.accountId} value={account.accountId}>
                  {account.customerPartyName ? `${account.customerPartyName} · ${account.accountId}` : account.accountId}
                </option>
              ))}
            </select>
          </label>
          <Input className="h-7 w-40 font-mono type-body" value={frame.owner} onChange={(event) => frame.setOwner(event.target.value)} placeholder="货主账户编码" />
          <WmsFilterSelect
            label="仓库"
            value={frame.warehouse}
            onChange={frame.setWarehouse}
            options={frame.lookups.warehouses.map((warehouse) => ({
              value: warehouse.warehouseCode,
              label: warehouse.name ? `${warehouse.name} · ${warehouse.warehouseCode}` : warehouse.warehouseCode,
            }))}
          />
        </>
      }
      columns={columns}
      rows={rows}
      rowKey={rowKey}
      selectedKey={frame.openKey}
      onSelect={frame.setOpenKey}
      rowAlign="baseline"
      noMatchText={noMatch}
      renderDetail={renderDetail}
      viewState={frame.viewState}
      listFooter={frame.pager ? <WmsPagerBar {...frame.pager} /> : undefined}
    />
  );
}

function stepButton(label: string, onClick: () => void) {
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        onClick();
      }}
    >
      {label}
    </Button>
  );
}

function SideDetail({ kind, code, badge, subtitle, children, onClose }: { kind: string; code: string; badge?: ReactNode; subtitle?: string; children: ReactNode; onClose: () => void }) {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-idpxyz-editor">
      <div className="flex shrink-0 items-start justify-between gap-2 border-b border-idpxyz-border px-4 py-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="type-section font-semibold text-idpxyz-textBright">{kind}</span>
            <span className="font-mono">{code}</span>
            {badge}
          </div>
          {subtitle ? <p className="mt-1 type-caption text-idpxyz-textMuted">{subtitle}</p> : null}
        </div>
        <Button type="button" variant="ghost" size="icon" title="关闭详情" aria-label="关闭详情" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>
      <div className="min-h-0 flex-1 space-y-3 overflow-auto p-4 type-body">{children}</div>
    </div>
  );
}

function CellSheet({ frame, cells }: { frame: Frame; cells: readonly Cell[] }) {
  const owner = frame.owner.trim();
  const columns: WorkbenchColumn<Cell & { key: string }>[] = [
    {
      key: 'place',
      header: '库位',
      width: fitColumnWidth,
      render: (row) =>
        row.location ? (
          <WmsTwoLine primary={<span className="font-mono">{row.location}</span>} secondary={nameOf(frame.warehouseNames, row.warehouse) || row.warehouse} code title={`${row.warehouse} / ${row.location}`} />
        ) : (
          <WmsTwoLine primary="代理仓，无库位" secondary={row.warehouse} code />
        ),
    },
    {
      key: 'goods',
      header: '商品',
      render: (row) => <WmsTwoLine fill primary={<span className="font-mono">{row.goodsCode}</span>} secondary={row.labelAlias} title={row.goodsCode} />,
    },
    { key: 'state', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.stockState} /> },
    { key: 'qty', header: nowrap('在库'), right: true, width: fitColumnWidth, render: (row) => <Qty value={row.quantity} kind="onHand" /> },
    { key: 'frozen', header: nowrap('冻着'), right: true, width: fitColumnWidth, minListWidth: 980, render: (row) => <Qty value={row.frozen} kind="frozen" /> },
    { key: 'available', header: nowrap('可发'), right: true, width: fitColumnWidth, render: (row) => <Qty value={row.available} kind="free" onHand={row.quantity} /> },
    { key: 'age', header: nowrap('库龄'), right: true, width: fitColumnWidth, minListWidth: 1120, render: (row) => nowrap(ageText(row.ageDays, row.age)) },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) => {
        const step = cellStep(frame, row);
        return step ? stepButton(step.label, step.run) : <span className="text-idpxyz-textMuted">—</span>;
      },
    },
  ];
  const rows = cells.map((cell, index) => ({ ...cell, key: `${cell.warehouse}/${cell.location ?? ''}/${cell.goodsCode}/${cell.inboundBatchId}/${index}` }));
  return (
    <StockFrame
      frame={frame}
      columns={columns}
      rows={rows}
      rowKey={(row) => row.key}
      noMatch="当前条件下没有商品库存格"
      renderDetail={(key) => {
        const row = rows.find((item) => item.key === key);
        if (!row) return <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这一格。</p>;
        return (
          <SideDetail kind="库存格" code={row.goodsCode} badge={<WmsBadge code={row.stockState} />} subtitle={row.location ?? '代理仓，无库位'} onClose={() => frame.setOpenKey(null)}>
            <p>{named(frame.warehouseNames, row.warehouse)}</p>
            <p>
              在库 <Qty value={row.quantity} kind="onHand" /> · 冻着 <Qty value={row.frozen} kind="frozen" /> · 可发{' '}
              <Qty value={row.available} kind="free" onHand={row.quantity} /> {row.unit}
            </p>
            <p className="font-mono">入库批次 {row.inboundBatchId}</p>
            <p className="font-mono">批号 {row.batchNumber || '—'} · 效期 {row.expiryDate || '—'}</p>
            {row.labelAlias ? <p className="font-mono">贴标 {row.labelAlias}</p> : null}
            <p>库龄 {ageText(row.ageDays, row.age)}</p>
            <div className="flex flex-wrap gap-2">
              {frame.can('stock-freeze') && row.location ? stepButton('冻结', () => frame.openAction('stock-freeze', freezeCell(owner, row))) : null}
              {frame.can('condition-restore') && row.stockState === 'DEFECTIVE' && row.location
                ? stepButton('次转良', () => frame.openAction('condition-restore', restoreCell(owner, row)))
                : null}
              {frame.can('adjustment-submit') && row.location ? stepButton('调账', () => frame.openAction('adjustment-submit', adjustCell(owner, row))) : null}
            </div>
          </SideDetail>
        );
      }}
    />
  );
}

function cellStep(frame: Frame, row: Cell): { label: string; run: () => void } | null {
  const owner = frame.owner.trim();
  if (!row.location) return null;
  if (frame.can('stock-freeze')) return { label: '冻结', run: () => frame.openAction('stock-freeze', freezeCell(owner, row)) };
  if (row.stockState === 'DEFECTIVE' && frame.can('condition-restore')) return { label: '次转良', run: () => frame.openAction('condition-restore', restoreCell(owner, row)) };
  if (frame.can('adjustment-submit')) return { label: '调账', run: () => frame.openAction('adjustment-submit', adjustCell(owner, row)) };
  return null;
}

function freezeCell(owner: string, cell: Cell): GuidedFormValues {
  return {
    warehouse: cell.warehouse,
    ownerAccount: owner,
    locationCode: cell.location ?? '',
    goodsCode: cell.goodsCode,
    unit: cell.unit,
    stockState: cell.stockState,
    inboundBatchId: cell.inboundBatchId,
    quantity: String(Math.max(1, cell.available)),
  };
}

function restoreCell(owner: string, cell: Cell): GuidedFormValues {
  return { ...freezeCell(owner, cell), stockState: 'DEFECTIVE' };
}

function adjustCell(owner: string, cell: Cell): GuidedFormValues {
  return {
    warehouse: cell.warehouse,
    ownerAccount: owner,
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
  };
}

function CartonSheet({ frame, cartons }: { frame: Frame; cartons: readonly Carton[] }) {
  const owner = frame.owner.trim();
  const columns: WorkbenchColumn<Carton>[] = [
    { key: 'carton', header: '箱号', width: fitColumnWidth, render: (row) => <span className="whitespace-nowrap font-mono">{row.cartonNumber}</span> },
    {
      key: 'place',
      header: '库位',
      width: fitColumnWidth,
      render: (row) => (
        <WmsTwoLine
          primary={<span className="font-mono">{row.location ?? '—'}</span>}
          secondary={nameOf(frame.warehouseNames, row.warehouse) || row.warehouse}
          code
        />
      ),
    },
    {
      key: 'bill',
      header: '箱内清单',
      render: (row) => {
        const text = row.bill && row.bill.length > 0 ? row.bill.map((line) => `${line.goodsCode}×${line.quantity}`).join('，') : '未报清单';
        return <WmsTwoLine fill primary={text} title={text} />;
      },
    },
    { key: 'held', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge label={row.held ? '已冻结' : '可用'} tone={row.held ? 'warning' : 'success'} /> },
    { key: 'age', header: nowrap('库龄'), right: true, width: fitColumnWidth, render: (row) => nowrap(ageText(row.ageDays, row.age)) },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) =>
        frame.can('stock-freeze') && !row.held
          ? stepButton('冻结整箱', () => frame.openAction('stock-freeze', { warehouse: row.warehouse, ownerAccount: owner, cartonNumber: row.cartonNumber }))
          : <span className="text-idpxyz-textMuted">—</span>,
    },
  ];
  return (
    <StockFrame
      frame={frame}
      columns={columns}
      rows={[...cartons]}
      rowKey={(row) => `${row.warehouse}/${row.cartonNumber}`}
      noMatch="当前条件下没有整箱库存"
      renderDetail={(key) => {
        const row = cartons.find((item) => `${item.warehouse}/${item.cartonNumber}` === key);
        if (!row) return <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这只箱。</p>;
        const bill = row.bill && row.bill.length > 0 ? row.bill.map((line) => `${line.goodsCode}×${line.quantity}`).join('，') : '未报清单';
        return (
          <SideDetail kind="整箱" code={row.cartonNumber} badge={<WmsBadge label={row.held ? '已冻结' : '可用'} tone={row.held ? 'warning' : 'success'} />} subtitle={row.location ?? '—'} onClose={() => frame.setOpenKey(null)}>
            <p>{named(frame.warehouseNames, row.warehouse)}</p>
            <p>{bill}</p>
            <p className="font-mono type-caption text-idpxyz-textMuted">入库 {formatInstant(row.inboundAt)} · 库龄 {ageText(row.ageDays, row.age)}</p>
            {frame.can('stock-freeze') && !row.held
              ? stepButton('冻结整箱', () => frame.openAction('stock-freeze', { warehouse: row.warehouse, ownerAccount: owner, cartonNumber: row.cartonNumber }))
              : null}
          </SideDetail>
        );
      }}
    />
  );
}

function SummarySheet({ frame, summary }: { frame: Frame; summary: readonly Summary[] }) {
  const columns: WorkbenchColumn<Summary & { key: string }>[] = [
    { key: 'goods', header: '商品', width: fitColumnWidth, render: (row) => <span className="whitespace-nowrap font-mono">{row.goodsCode}</span> },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, render: (row) => named(frame.warehouseNames, row.warehouse) },
    { key: 'state', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.stockState} /> },
    { key: 'unit', header: '单位', width: fitColumnWidth, minListWidth: 900, render: (row) => nowrap(row.unit) },
    { key: 'onHand', header: nowrap('在库'), right: true, width: fitColumnWidth, render: (row) => <Qty value={row.onHand} kind="onHand" /> },
    { key: 'allocatable', header: nowrap('可分配'), right: true, width: fitColumnWidth, minListWidth: 980, render: (row) => <Qty value={row.allocatable} kind="allocatable" /> },
    { key: 'frozen', header: nowrap('冻着'), right: true, width: fitColumnWidth, minListWidth: 1080, render: (row) => <Qty value={row.frozen} kind="frozen" /> },
    { key: 'reserved', header: nowrap('已预占'), right: true, width: fitColumnWidth, minListWidth: 1160, render: (row) => <Qty value={row.reserved} kind="reserved" /> },
    { key: 'locked', header: nowrap('锁着'), right: true, width: fitColumnWidth, minListWidth: 1240, render: (row) => <Qty value={row.locked} kind="locked" /> },
    { key: 'available', header: nowrap('可发'), right: true, width: fitColumnWidth, render: (row) => <Qty value={row.available} kind="free" onHand={row.onHand} /> },
  ];
  const rows = summary.map((row, index) => ({ ...row, key: `${row.warehouse}/${row.goodsCode}/${row.unit}/${row.stockState}/${index}` }));
  return (
    <StockFrame
      frame={frame}
      columns={columns}
      rows={rows}
      rowKey={(row) => row.key}
      noMatch="当前条件下没有汇总"
      renderDetail={(key) => {
        const row = rows.find((item) => item.key === key);
        if (!row) return <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这一行。</p>;
        return (
          <SideDetail kind="库存汇总" code={row.goodsCode} badge={<WmsBadge code={row.stockState} />} subtitle={row.unit} onClose={() => frame.setOpenKey(null)}>
            <p>{named(frame.warehouseNames, row.warehouse)}</p>
            <p>
              在库 <Qty value={row.onHand} kind="onHand" /> · 可分配 <Qty value={row.allocatable} kind="allocatable" /> · 冻着 <Qty value={row.frozen} kind="frozen" />
            </p>
            <p>
              已预占 <Qty value={row.reserved} kind="reserved" /> · 锁着 <Qty value={row.locked} kind="locked" /> · 可发{' '}
              <Qty value={row.available} kind="free" onHand={row.onHand} />
            </p>
          </SideDetail>
        );
      }}
    />
  );
}

function HoldSheet({ frame, holds }: { frame: Frame; holds: readonly Hold[] }) {
  const columns: WorkbenchColumn<Hold & { key: string }>[] = [
    { key: 'kind', header: '类型', width: fitColumnWidth, render: (row) => nowrap(holdKindLabels[row.kind] ?? row.kind) },
    {
      key: 'place',
      header: '库位',
      width: fitColumnWidth,
      render: (row) => (
        <WmsTwoLine primary={<span className="font-mono">{row.location ?? '—'}</span>} secondary={nameOf(frame.warehouseNames, row.warehouse) || row.warehouse} code />
      ),
    },
    {
      key: 'goods',
      header: '商品 / 整箱',
      render: (row) =>
        row.cartonNumber ? (
          <WmsTwoLine primary="整箱" secondary={row.cartonNumber} code />
        ) : (
          <WmsTwoLine primary={<span className="font-mono">{row.goodsCode ?? '—'}</span>} secondary={row.unit} />
        ),
    },
    {
      key: 'qty',
      header: nowrap('数量'),
      right: true,
      width: fitColumnWidth,
      render: (row) => <Qty value={row.quantity} kind={row.kind === 'RESERVED' ? 'reserved' : 'frozen'} />,
    },
    {
      key: 'reason',
      header: '原因',
      minListWidth: 1000,
      render: (row) => <WmsTwoLine fill primary={row.reason || '—'} secondary={row.operator} title={row.reason ?? ''} />,
    },
  ];
  const rows = holds.map((hold, index) => ({ ...hold, key: `${hold.kind}/${hold.warehouse}/${hold.goodsCode ?? hold.cartonNumber ?? ''}/${index}` }));
  return (
    <StockFrame
      frame={frame}
      columns={columns}
      rows={rows}
      rowKey={(row) => row.key}
      noMatch="当前没有冻结或占用记录"
      renderDetail={(key) => {
        const row = rows.find((item) => item.key === key);
        if (!row) return <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这一条。</p>;
        return (
          <SideDetail kind={holdKindLabels[row.kind] ?? row.kind} code={row.cartonNumber ?? row.goodsCode ?? '—'} subtitle={row.location ?? '—'} onClose={() => frame.setOpenKey(null)}>
            <p>{named(frame.warehouseNames, row.warehouse)}</p>
            <p>
              数量 <Qty value={row.quantity} kind={row.kind === 'RESERVED' ? 'reserved' : 'frozen'} /> {row.unit ?? ''}
            </p>
            <p>原因 {row.reason || '—'}</p>
            <p className="font-mono type-caption text-idpxyz-textMuted">操作者 {row.operator || '—'}{row.frozenAt ? ` · ${formatInstant(row.frozenAt)}` : ''}</p>
          </SideDetail>
        );
      }}
    />
  );
}

function MovementSheet({ frame, movements }: { frame: Frame; movements: readonly Movement[] }) {
  const now = new Date();
  const columns: WorkbenchColumn<Movement & { key: string }>[] = [
    { key: 'warehouse', header: '仓', width: fitColumnWidth, render: (row) => named(frame.warehouseNames, row.warehouse) },
    { key: 'reason', header: '原因', render: (row) => <WmsTwoLine fill primary={labelOf(row.reason)} title={row.reason} /> },
    { key: 'source', header: '来源', width: fitColumnWidth, minListWidth: 980, render: (row) => <span className="whitespace-nowrap font-mono">{row.source ?? '—'}</span> },
    {
      key: 'at',
      header: nowrap('发生'),
      right: true,
      width: fitColumnWidth,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.occurredAt)}>
          {shortInstant(row.occurredAt, now) || '—'}
        </span>
      ),
    },
  ];
  const rows = movements.map((movement, index) => ({ ...movement, key: `${movement.warehouse}/${movement.source ?? ''}/${movement.occurredAt}/${index}` }));
  return (
    <StockFrame
      frame={frame}
      columns={columns}
      rows={rows}
      rowKey={(row) => row.key}
      noMatch="当前条件下没有库存变动流水"
      renderDetail={(key) => {
        const row = rows.find((item) => item.key === key);
        if (!row) return <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这一笔。</p>;
        return (
          <SideDetail kind="库存流水" code={labelOf(row.reason)} subtitle={row.source ?? '—'} onClose={() => frame.setOpenKey(null)}>
            <p>{named(frame.warehouseNames, row.warehouse)}</p>
            <p className="font-mono type-caption text-idpxyz-textMuted">发生 {formatInstant(row.occurredAt)}{row.postedAt ? ` · 入账 ${formatInstant(row.postedAt)}` : ''}</p>
            <ul className="space-y-1 font-mono type-caption">
              {(row.lines ?? []).map((line, index) => (
                <li key={index}>
                  {`${line.location ?? '仓'} · ${line.goodsCode} ${line.unit} ${labelOf(line.stockState)} `}
                  <Qty value={line.delta} kind="delta" />
                </li>
              ))}
            </ul>
          </SideDetail>
        );
      }}
    />
  );
}
