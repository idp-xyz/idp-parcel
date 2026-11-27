import { useEffect, useMemo, useState, type ComponentProps, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button, Tabs, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
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
import { listOwnerGoods, queryOwnerStock, type ApiResult, type StockReadBody, type StockViewKind } from './api';
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
  FREEZE: '手动冻结',
  RESERVATION: '出库预占',
  LOCKED: '作业锁',
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
  // 在库和可发是扫表时要抓住的两个数，有数就加重。其余数量有数时略重。零保持原重。不改字号，改了会把一行的基线撑开。
  const weight = value === 0 ? '' : kind === 'onHand' || kind === 'free' || kind === 'delta' ? 'font-semibold' : 'font-medium';
  const text = kind === 'delta' && value > 0 ? `+${value}` : String(value);
  return <span className={`whitespace-nowrap tabular-nums ${weight} ${tone}`}>{text}</span>;
}

function stateOptions(view: StockViewKind): { value: string; label: string }[] {
  if (view === 'cells' || view === 'summary') {
    return [
      { value: 'GOOD', label: '良品' },
      { value: 'PENDING_INSPECTION', label: '待检' },
      { value: 'DEFECTIVE', label: '次品' },
    ];
  }
  if (view === 'cartons') {
    return [
      { value: 'FREE', label: '可用' },
      { value: 'HELD', label: '已冻结' },
    ];
  }
  if (view === 'holds') {
    return [
      { value: 'FREEZE', label: '手动冻结' },
      { value: 'CARTON', label: '整箱冻结' },
      { value: 'RESERVATION', label: '出库预占' },
    ];
  }
  return [];
}

function keepStockRow(view: StockViewKind, row: Cell | Carton | Summary | Hold | Movement, state: string): boolean {
  if (view === 'cells' || view === 'summary') return 'stockState' in row && row.stockState === state;
  if (view === 'cartons') return 'held' in row && (state === 'HELD' ? row.held : !row.held);
  if (!('kind' in row)) return false;
  if (state === 'CARTON') return row.kind === 'FREEZE' && Boolean(row.cartonNumber);
  if (state === 'FREEZE') return row.kind === 'FREEZE' && !row.cartonNumber;
  return row.kind === state;
}

function listedOf(view: StockViewKind, body: StockReadBody | null): ReadonlyArray<Cell | Carton | Summary | Hold | Movement> {
  if (!body) return [];
  if (view === 'cells') return body.cells ?? [];
  if (view === 'cartons') return body.cartons ?? [];
  if (view === 'summary') return body.summary ?? [];
  if (view === 'holds') return body.holds ?? [];
  return body.movements ?? [];
}

/** 可发是 0、库里却有货时，右栏用一句话说明卡在哪。allocatable 在库存格上是能不能分配，在汇总上是可分配的数量。 */
function shipBlock(row: Cell): string | null {
  if (row.available > 0 || row.quantity <= 0) return null;
  if (row.frozen > 0) return '有货冻着，发不出。';
  if (row.allocatable === false) return '这个库位的货不参与分配，发不出。';
  if (row.stockState === 'PENDING_INSPECTION') return '还在待检，发不出。';
  if (row.stockState === 'DEFECTIVE') return '是次品。';
  return null;
}

function summaryBlock(row: Summary): string | null {
  if (row.available > 0 || row.onHand <= 0) return null;
  if (row.frozen > 0) return '有货冻着，发不出。';
  if (row.reserved > 0) return '已经预占给出库，发不出。';
  if (row.locked > 0) return '作业锁着，发不出。';
  if (row.allocatable === 0) return '没有可分配的数量，发不出。';
  return null;
}

function goodsLine(names: ReadonlyMap<string, string>, code: string, alias?: string) {
  const name = names.get(code);
  const codeLine = alias ? `${code} · ${alias}` : code;
  return name ? (
    <WmsTwoLine fill primary={name} secondary={<span className="font-mono">{codeLine}</span>} title={`${name} · ${codeLine}`} />
  ) : (
    <WmsTwoLine fill primary={<span className="font-mono">{code}</span>} secondary={alias} title={code} />
  );
}

function quantityWithUnit(value: number, kind: 'onHand' | 'free', unit: string, onHand = 0) {
  return (
    <span className="inline-flex items-baseline justify-end gap-1">
      <Qty value={value} kind={kind} onHand={onHand} />
      <span className="type-caption text-idpxyz-textMuted">{unit}</span>
    </span>
  );
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
  const [stateByView, setStateByView] = useTabViewState<Partial<Record<StockViewKind, string>>>('warehouse-stock.state', {});
  const [view, setView] = useTabViewState<StockViewKind>('warehouse-stock.view', 'cells');
  const [openByView, setOpenByView] = useTabViewState<Partial<Record<StockViewKind, string | null>>>('warehouse-stock.open', {});
  const [storedMovementSize, setMovementSize] = useTabViewState('warehouse-stock.movementSize', catalogueDefaultPageSize);
  const [storedSheetSize, setSheetSize] = useTabViewState('warehouse-stock.sheetSize', catalogueDefaultPageSize);
  const [slot, setSlot] = useState<{ scope: string; answer: ApiResult<StockReadBody> } | null>(null);
  const [loading, setLoading] = useState(false);
  const [tick, setTick] = useState(0);
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [goodsNames, setGoodsNames] = useState<ReadonlyMap<string, string>>(() => new Map());
  const [paging, setPaging] = useState({ conditions: '', page: 1 });
  const [trail, setTrail] = useState<CursorTrail>(() => startCursorTrail(''));

  const owner = storedOwner;
  const settled = useSettledKeyword(keyword);
  const movementSize = allowedSize(storedMovementSize);
  const sheetSize = allowedSize(storedSheetSize);
  const openKey = openByView[view] ?? null;
  const setOpenKey = (key: string | null) => setOpenByView((current) => ({ ...current, [view]: key }));
  const state = stateByView[view] ?? '';
  const setState = (value: string) => setStateByView((current) => ({ ...current, [view]: value }));
  const options = stateOptions(view);
  // 流水没有库存状态。选了状态就不再向读口要页码：读口不收状态，不带页码时答整份，页在这里切。
  const filtering = state !== '' && view !== 'movements';

  // 货主只从名册里选。名册还没人选、或标签里留着一个名册上没有的编码时，用第一位。
  useEffect(() => {
    if (!lookups.loaded || lookups.owners.length === 0) return;
    if (!lookups.owners.some((account) => account.accountId === owner)) setOwner(lookups.owners[0].accountId);
  }, [lookups.loaded, lookups.owners, owner, setOwner]);

  // 页码与游标跟着查阅条件：货主、仓、看法、检索词、状态或页大小一换就从第一页看起。流水按游标翻，其余四种按页码。
  const conditions = `${owner.trim()}|${warehouse}|${view}|${settled.trim()}|${state}|${view === 'movements' ? movementSize : sheetSize}`;
  if (trail.query !== conditions) {
    setTrail(startCursorTrail(conditions));
    setPaging({ conditions, page: 1 });
  }
  const cursor = cursorTrailFor(trail, conditions);
  const after = view === 'movements' ? currentCursorAfter(cursor) : null;
  const sheetPage = paging.conditions === conditions ? paging.page : 1;
  const scope = `${owner.trim()}|${warehouse}|${view}|${settled.trim()}|${state}`;
  const serverSize = view === 'movements' ? movementSize : filtering ? undefined : sheetSize;
  const serverPage = view === 'movements' || filtering ? undefined : sheetPage;

  useEffect(() => {
    const trimmed = owner.trim();
    if (!trimmed) {
      setGoodsNames(new Map());
      return;
    }
    let cancelled = false;
    void listOwnerGoods(trimmed, '', null, 200).then((res) => {
      if (cancelled || res.kind !== 'outcome') return;
      setGoodsNames(new Map(res.body.goods.map((item) => [item.goodsCode, item.name])));
    });
    return () => {
      cancelled = true;
    };
  }, [owner]);

  useEffect(() => {
    const trimmed = owner.trim();
    if (!trimmed) return;
    let cancelled = false;
    setLoading(true);
    void queryOwnerStock(trimmed, warehouse, view, after, serverSize, serverPage, settled).then((res) => {
      if (!cancelled) {
        setSlot({ scope, answer: res });
        setLoading(false);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [owner, warehouse, view, settled, tick, after, serverSize, serverPage, scope]);

  const answer = slot && slot.scope === scope ? slot.answer : null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const movementPage = body?.page ?? null;
  const movementPageNumber = cursorPageNumber(cursor);
  const book = body?.book ?? null;
  const listed = listedOf(view, body);
  const filtered = filtering ? listed.filter((row) => keepStockRow(view, row, state)) : listed;
  const shown = filtering ? filtered.slice((sheetPage - 1) * sheetSize, sheetPage * sheetSize) : filtered;
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
          summary: cursorPageSummary(filtering ? sheetPage : (book?.page ?? sheetPage), filtering ? sheetSize : (book?.pageSize ?? sheetSize), filtering ? filtered.length : (book?.total ?? null)),
          canPrevious: !loading && sheetPage > 1,
          canNext: !loading && (filtering ? sheetPage * sheetSize < filtered.length : book !== null && sheetPage * (book.pageSize || sheetSize) < book.total),
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

  const narrowed = warehouse !== '' || settled.trim() !== '' || state !== '' || (view === 'movements' ? movementPageNumber > 1 : sheetPage > 1);
  const rowCount = filtering ? filtered.length : view === 'movements' ? (body?.movements?.length ?? 0) : listed.length;
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
            emptyDescription: '换个商品、库位、箱号或状态，或换一个货主、仓库再查。流水只保留已经入账的变动。',
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
    state,
    setState,
    stateOptions: options,
    goodsNames,
    unfreeze: can('stock-unfreeze') ? (
      <Button type="button" variant="outline" onClick={() => openAction('stock-unfreeze')}>
        按冻结号解冻
      </Button>
    ) : undefined,
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      {view === 'cells' ? <CellSheet frame={frame} cells={shown as readonly Cell[]} /> : null}
      {view === 'cartons' ? <CartonSheet frame={frame} cartons={shown as readonly Carton[]} /> : null}
      {view === 'summary' ? <SummarySheet frame={frame} summary={shown as readonly Summary[]} /> : null}
      {view === 'holds' ? <HoldSheet frame={frame} holds={shown as readonly Hold[]} /> : null}
      {view === 'movements' ? <MovementSheet frame={frame} movements={shown as readonly Movement[]} /> : null}
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
  state: string;
  setState: (value: string) => void;
  stateOptions: ReadonlyArray<{ value: string; label: string }>;
  goodsNames: ReadonlyMap<string, string>;
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
              className="h-7 max-w-[24rem] rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text focus:border-idpxyz-accent focus:outline-none"
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
          {frame.stateOptions.length > 0 ? (
            <WmsFilterSelect label="状态" value={frame.state} onChange={frame.setState} options={frame.stateOptions} />
          ) : null}
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
      render: (row) => goodsLine(frame.goodsNames, row.goodsCode, row.labelAlias),
    },
    { key: 'state', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.stockState} /> },
    { key: 'qty', header: nowrap('在库'), right: true, width: fitColumnWidth, render: (row) => quantityWithUnit(row.quantity, 'onHand', row.unit) },
    { key: 'frozen', header: nowrap('冻着'), right: true, width: fitColumnWidth, render: (row) => <Qty value={row.frozen} kind="frozen" /> },
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
            {shipBlock(row) ? <p>{shipBlock(row)}</p> : null}
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
    { key: 'goods', header: '商品', render: (row) => goodsLine(frame.goodsNames, row.goodsCode) },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, render: (row) => named(frame.warehouseNames, row.warehouse) },
    { key: 'state', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.stockState} /> },
    { key: 'onHand', header: nowrap('在库'), right: true, width: fitColumnWidth, render: (row) => quantityWithUnit(row.onHand, 'onHand', row.unit) },
    { key: 'allocatable', header: nowrap('可分配'), right: true, width: fitColumnWidth, minListWidth: 1100, render: (row) => <Qty value={row.allocatable} kind="allocatable" /> },
    { key: 'frozen', header: nowrap('冻着'), right: true, width: fitColumnWidth, render: (row) => <Qty value={row.frozen} kind="frozen" /> },
    { key: 'reserved', header: nowrap('已预占'), right: true, width: fitColumnWidth, render: (row) => <Qty value={row.reserved} kind="reserved" /> },
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
            {summaryBlock(row) ? <p>{summaryBlock(row)}</p> : null}
          </SideDetail>
        );
      }}
    />
  );
}

function HoldSheet({ frame, holds }: { frame: Frame; holds: readonly Hold[] }) {
  const columns: WorkbenchColumn<Hold & { key: string }>[] = [
    {
      key: 'kind',
      header: '类型',
      width: fitColumnWidth,
      render: (row) => nowrap(row.kind === 'FREEZE' && row.cartonNumber ? '整箱冻结' : (holdKindLabels[row.kind] ?? row.kind)),
    },
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
        ) : row.goodsCode ? (
          goodsLine(frame.goodsNames, row.goodsCode)
        ) : (
          <span className="text-idpxyz-textMuted">—</span>
        ),
    },
    {
      key: 'qty',
      header: nowrap('数量'),
      right: true,
      width: fitColumnWidth,
      render: (row) => <Qty value={row.quantity} kind={row.kind === 'RESERVATION' ? 'reserved' : 'frozen'} />,
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
          <SideDetail kind={row.kind === 'FREEZE' && row.cartonNumber ? '整箱冻结' : (holdKindLabels[row.kind] ?? row.kind)} code={row.cartonNumber ?? row.goodsCode ?? '—'} subtitle={row.location ?? '—'} onClose={() => frame.setOpenKey(null)}>
            <p>{named(frame.warehouseNames, row.warehouse)}</p>
            <p>
              数量 <Qty value={row.quantity} kind={row.kind === 'RESERVATION' ? 'reserved' : 'frozen'} /> {row.unit ?? ''}
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
    { key: 'reason', header: '原因', width: fitColumnWidth, render: (row) => <WmsTwoLine primary={labelOf(row.reason)} title={row.reason} /> },
    {
      key: 'lines',
      header: '变动',
      render: (row) => {
        const lines = row.lines ?? [];
        const first = lines[0];
        if (!first) return <span className="text-idpxyz-textMuted">—</span>;
        const name = frame.goodsNames.get(first.goodsCode);
        return (
          <WmsTwoLine
            fill
            primary={
              <span className="inline-flex items-baseline gap-2">
                <span>{name ?? first.goodsCode}</span>
                <Qty value={first.delta} kind="delta" />
              </span>
            }
            secondary={lines.length > 1 ? `另 ${lines.length - 1} 行` : `${first.location ?? '仓'} · ${labelOf(first.stockState)}`}
            title={first.goodsCode}
          />
        );
      },
    },
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
