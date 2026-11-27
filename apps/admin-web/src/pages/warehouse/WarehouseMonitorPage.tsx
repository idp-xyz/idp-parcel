import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, type WorkbenchColumn } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { moduleInfoById } from '../../navigation';
import { catalogueDefaultPageSize, cataloguePageSizes } from '../catalogue-query';
import { cursorPageSummary } from '../../templates/cursor-pagination';
import {
  getOutboundRequestDetail,
  getWave,
  listAdjustments,
  listCounts,
  listFreezes,
  listOutboundRequests,
  listOwnerPreAdvices,
  listUnclaimedArrivals,
  listWaves,
  queryOwnerStock,
} from './api';
import { documentHash, documentKeyParts } from './document-address';
import { AdjustmentDocument } from './AdjustmentDocument';
import { CountDocument } from './CountDocument';
import { DocumentLink } from './document-page';
import { adjustmentKind, countKind, countsAdjustmentsModule } from './counts-adjustments-address';
import { FreezeDocument } from './FreezeDocument';
import { fitColumnWidth, nameOf } from './list-cells';
import { freezeKind, processingModule } from './processing-address';
import { OutboundDocument } from './OutboundDocument';
import { PreAdviceDocument } from './PreAdviceDocument';
import { StockDetail } from './stock-detail';
import { UnclaimedDocument } from './UnclaimedDocument';
import { useWarehouseLookups } from './use-catalogues';
import { WaveDocument } from './WaveDocument';
import { labelOf, WmsBadge, WmsFilterSelect, WmsInfoGrid, WmsPagerBar, WmsTwoLine } from './wms-ui';
import {
  monitorCursorCanPrevious,
  monitorCursorNext,
  monitorCursorPrevious,
  monitorCursorStart,
  type MonitorCursor,
} from './monitor-page';
import {
  monitorFilters,
  monitorGroups,
  monitorNodes,
  outboundHeaders,
  outboundLines,
  taskHeaders,
  taskLinesFromDetail,
  waveHeaders,
  waveLines,
  waveTaskLines,
  type MonitorLine,
  type MonitorNode,
} from './monitor-tree';

const info = moduleInfoById['warehouse-monitor'];
const opening = monitorNodes.find((item) => item.id === 'outbound') ?? monitorNodes[0];
const numericHeaders = new Set(['应拣', '已拣', '在库', '出库单', '任务']);

interface View {
  title: string;
  headers: string[];
  lines: MonitorLine[];
  note: string;
  next: string | null;
  total?: number | null;
  book?: { page: number; pageSize: number; total: number } | null;
}

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/**
 * 仓库监控。一次只看一种有列表读口的对象，对象在工具条里换。
 * 点一行在右边开那张单据；库存格、整箱和钻下去的拣货任务没有单据页，右边只摆这一行。
 * 行上的「拣货任务」「这一格」往下看，不另开标签。
 */
export function WarehouseMonitorPage() {
  const lookups = useWarehouseLookups();
  const listed = monitorGroups().flatMap((group) => group.nodes);
  const [storedNode, setNodeId] = useTabViewState('warehouse-monitor.node', opening.id);
  const node = listed.find((item) => item.id === storedNode) ?? opening;
  const [warehouse, setWarehouse] = useTabViewState('warehouse-monitor.warehouse', '');
  const [owner, setOwner] = useTabViewState('warehouse-monitor.owner', '');
  const [status, setStatus] = useTabViewState('warehouse-monitor.status', '');
  const [nonce, setNonce] = useState(0);
  const [cursor, setCursor] = useState<MonitorCursor>(monitorCursorStart);
  const [pageSize, setPageSize] = useState(catalogueDefaultPageSize);
  const [sheetPage, setSheetPage] = useState(1);
  const [stack, setStack] = useState<View[]>([]);
  const [view, setView] = useState<View>({ title: opening.label, headers: [], lines: [], note: '', next: null });
  const [loading, setLoading] = useState(true);
  const [openKey, setOpenKey] = useState<string | null>(null);
  const generation = useRef(0);
  const statusOk = node.status?.includes(status) ?? status === '';
  const effectiveStatus = statusOk ? status : '';
  const needsOwner = Boolean(node.needsOwner) && owner.trim() === '';

  useEffect(() => {
    setLoading(true);
    setStack([]);
    setView({ title: node.label, headers: [], lines: [], note: '', next: null });
    const timer = setTimeout(() => {
      const mine = ++generation.current;
      void (async () => {
        const next = await readView(node, warehouse, owner, effectiveStatus, cursor.after, pageSize, sheetPage);
        if (mine !== generation.current) return;
        setView(next);
        setLoading(false);
      })();
    }, 250);
    return () => clearTimeout(timer);
  }, [node, warehouse, owner, effectiveStatus, nonce, cursor.after, pageSize, sheetPage]);

  const ownerNames = useMemo(
    () => new Map(lookups.owners.map((item) => [item.accountId, item.customerPartyName ?? ''])),
    [lookups.owners],
  );
  const warehouseNames = useMemo(
    () => new Map(lookups.warehouses.map((item) => [item.warehouseCode, item.name ?? ''])),
    [lookups.warehouses],
  );

  const drill = async (line: MonitorLine) => {
    // 往下看时作废还在途的顶层读取，免得它晚到把这一层盖掉，刷新标也不该一直转。
    generation.current += 1;
    setLoading(false);
    setOpenKey(null);
    if (line.drillWave) {
      const answer = await getWave(line.drillWave);
      if (answer.kind !== 'outcome' || !answer.body.wave) {
        setView({ ...view, note: '这个波次没读成。' });
        return;
      }
      const tasks = answer.body.wave.tasks ?? [];
      setStack((current) => [...current, view]);
      setView({
        title: `${line.cells[0]} 的拣货任务`,
        headers: taskHeaders,
        lines: waveTaskLines(tasks),
        note: tasks.length === 0 ? '这个波次没有拣货任务。' : '波次上的任务没有货主，这一层不再钻库存格。要从出库单钻下去。',
        next: null,
      });
      return;
    }
    if (line.drillOutbound) {
      const answer = await getOutboundRequestDetail(line.drillOutbound);
      if (answer.kind !== 'outcome' || !answer.body.request) {
        setView({ ...view, note: '这张出库单没读成。' });
        return;
      }
      const request = answer.body.request;
      const tasks = answer.body.pickTasks ?? [];
      setStack((current) => [...current, view]);
      setView({
        title: `${request.documentNumber ?? request.requestId} 的拣货任务`,
        headers: taskHeaders,
        lines: taskLinesFromDetail(tasks, request.ownerAccount, request.warehouseCode),
        note: tasks.length === 0 ? '这张单还没有拣货任务。' : '点「这一格」看这个库位上的这件货。',
        next: null,
      });
      return;
    }
    const stock = line.drillStock;
    if (!stock) return;
    const answer = await queryOwnerStock(stock.owner, stock.warehouse, 'cells');
    if (answer.kind !== 'outcome') {
      setView({ ...view, note: '库存格没读成。' });
      return;
    }
    const cells = (answer.body.cells ?? []).filter((cell) => cell.location === stock.location && cell.goodsCode === stock.goods);
    setStack((current) => [...current, view]);
    setView({
      title: `${stock.location} · ${stock.goods}`,
      headers: ['库位', '商品', '状态', '在库'],
      lines: cells.map((cell) => ({ key: `${cell.location}|${cell.goodsCode}|${cell.inboundBatchId}`, cells: [cell.location ?? '', cell.goodsCode, cell.stockState, String(cell.quantity)] })),
      note: cells.length === 0 ? '这个库位上没有这件货的库存格。' : '按货主取回这一仓的库存格，再留下这一库位上的这件货。',
      next: null,
    });
  };

  const choose = (item: MonitorNode) => {
    setNodeId(item.id);
    setStatus('');
    setCursor(monitorCursorStart());
    setSheetPage(1);
    setOpenKey(null);
    setStack([]);
  };

  const narrow = (apply: () => void) => {
    apply();
    setCursor(monitorCursorStart());
    setSheetPage(1);
  };

  const back = () => {
    const previous = stack[stack.length - 1];
    if (!previous) return;
    setStack(stack.slice(0, -1));
    setView(previous);
    setOpenKey(null);
  };

  const drilled = view.lines.some((line) => line.drillLabel);
  const columns: WorkbenchColumn<MonitorLine>[] = view.headers.map((header, index) => ({
    key: `${header}-${index}`,
    header: nowrap(header),
    right: numericHeaders.has(header),
    width: index === view.headers.length - 1 ? undefined : fitColumnWidth,
    render: (row) => monitorCell(header, row.cells[index] ?? '', row, index, ownerNames, warehouseNames),
  }));
  if (drilled) {
    columns.push({
      key: 'next',
      header: nowrap('往下'),
      width: fitColumnWidth,
      render: (row) =>
        row.drillLabel ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
            onClick={(event) => {
              event.stopPropagation();
              void drill(row);
            }}
          >
            {row.drillLabel}
          </Button>
        ) : (
          <span className="text-idpxyz-textMuted">—</span>
        ),
    });
  }

  // 换对象的这一帧，上一份列表还在 state 里。路径先跟当前对象走，等这一份读到再带上它的说明。
  const aligned = stack.length > 0 || view.title === node.label;
  const crumb = stack.length > 0 ? [...stack.map((item) => item.title), view.title].join(' / ') : node.label;
  const subtitle = aligned && !needsOwner && view.note !== '' ? `${crumb}。${view.note}` : crumb;
  const narrowed = warehouse !== '' || owner !== '' || effectiveStatus !== '';
  const clearFilters = () => {
    setWarehouse('');
    setOwner('');
    setStatus('');
    setCursor(monitorCursorStart());
    setSheetPage(1);
  };
  const readFailed = aligned && view.note.endsWith('没读成。');
  const viewState: TemplateViewState = needsOwner
    ? { kind: 'empty', title: '先选货主', description: '这个对象按货主取数。先选一位货主，再看。' }
    : !aligned || (loading && view.lines.length === 0)
      ? { kind: 'loading', shape: 'table' }
      : readFailed
        ? { kind: 'error', description: view.note, onRetry: () => setNonce((current) => current + 1) }
        : { kind: 'ready' };
  const showPager = stack.length === 0 && viewState.kind === 'ready';

  return (
    <WorkbenchPageTemplate<MonitorLine>
      title={info?.title ?? '仓库监控'}
      viewStateKey="warehouse-monitor"
      subtitle={subtitle}
      onRefresh={() => setNonce((current) => current + 1)}
      refreshing={loading}
      extraFilters={
        <>
          <label className="inline-flex items-center gap-1.5 type-body text-idpxyz-textMuted">
            <span className="whitespace-nowrap">对象</span>
            <select
              aria-label="对象"
              className="h-7 max-w-[14rem] rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text focus:border-idpxyz-accent focus:outline-none"
              value={node.id}
              onChange={(event) => choose(listed.find((item) => item.id === event.target.value) ?? opening)}
            >
              {monitorGroups().map((group) => (
                <optgroup key={group.group} label={group.group}>
                  {group.nodes.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.label}
                    </option>
                  ))}
                </optgroup>
              ))}
            </select>
          </label>
          {node.warehouse || node.id === 'pre-advice' ? (
            <WmsFilterSelect
              label="仓"
              value={warehouse}
              options={lookups.warehouses.map((item) => ({ value: item.warehouseCode, label: item.name ? item.name : item.warehouseCode }))}
              onChange={(next) => narrow(() => setWarehouse(next))}
            />
          ) : null}
          {node.owner || node.needsOwner ? (
            <WmsFilterSelect
              label="货主"
              value={owner}
              options={lookups.owners.map((item) => ({ value: item.accountId, label: item.customerPartyName ? item.customerPartyName : item.accountId }))}
              onChange={(next) => narrow(() => setOwner(next))}
            />
          ) : null}
          {node.status ? (
            <WmsFilterSelect
              label="状态"
              value={effectiveStatus}
              options={node.status.map((code) => ({ value: code, label: labelOf(code, code) }))}
              onChange={(next) => narrow(() => setStatus(next))}
            />
          ) : null}
          {stack.length > 0 ? (
            <Button type="button" variant="outline" size="sm" onClick={back}>
              返回上一层
            </Button>
          ) : null}
          {narrowed ? (
            <button
              type="button"
              className="whitespace-nowrap type-body text-idpxyz-textMuted hover:text-idpxyz-textBright focus-visible:underline focus-visible:outline-none"
              onClick={clearFilters}
            >
              清除
            </button>
          ) : null}
        </>
      }
      columns={columns}
      rows={view.lines}
      rowKey={(row) => row.key}
      selectedKey={needsOwner ? null : openKey}
      onSelect={setOpenKey}
      rowAlign="baseline"
      noMatchText={view.note || '这一页没有。'}
      onClearFilters={narrowed ? clearFilters : undefined}
      renderDetail={(key) => (
        <MonitorDetail
          documentNode={stack.length === 0 ? node.id : ''}
          owner={owner}
          lineKey={key}
          line={view.lines.find((row) => row.key === key)}
          headers={view.headers}
          onClose={() => setOpenKey(null)}
          onChanged={() => setNonce((current) => current + 1)}
          revision={nonce}
        />
      )}
      viewState={viewState}
      listFooter={
        showPager ? (
          <WmsPagerBar
            summary={
              monitorIsSheet(node)
                ? cursorPageSummary(view.book?.page ?? sheetPage, pageSize, view.book?.total ?? null)
                : cursorPageSummary(cursor.trail.length, pageSize, view.total ?? null)
            }
            canPrevious={!loading && (monitorIsSheet(node) ? sheetPage > 1 : monitorCursorCanPrevious(cursor))}
            canNext={!loading && (monitorIsSheet(node) ? view.book != null && sheetPage * pageSize < view.book.total : view.next !== null)}
            onPrevious={() => (monitorIsSheet(node) ? setSheetPage((current) => Math.max(1, current - 1)) : setCursor(monitorCursorPrevious(cursor)))}
            onNext={() => {
              if (monitorIsSheet(node)) {
                setSheetPage((current) => current + 1);
                return;
              }
              const next = view.next;
              if (next !== null) setCursor(monitorCursorNext(cursor, next));
            }}
            pageSize={
              !node.unavailable
                ? {
                    value: pageSize,
                    options: cataloguePageSizes,
                    onChange: (next) => {
                      setPageSize(next);
                      setCursor(monitorCursorStart());
                      setSheetPage(1);
                    },
                  }
                : undefined
            }
          />
        ) : undefined
      }
    />
  );
}

function MonitorDetail({
  documentNode,
  owner,
  lineKey,
  line,
  headers,
  onClose,
  onChanged,
  revision,
}: {
  documentNode: string;
  owner: string;
  lineKey: string;
  line: MonitorLine | undefined;
  headers: readonly string[];
  onClose: () => void;
  onChanged: () => void;
  revision: number;
}) {
  const panel = { onClose, onChanged, revision };
  if (documentNode === 'outbound') return <OutboundDocument requestId={lineKey} panel={panel} />;
  if (documentNode === 'waves') return <WaveDocument waveId={lineKey} panel={panel} />;
  if (documentNode === 'unclaimed') return <UnclaimedDocument arrivalId={lineKey} panel={panel} />;
  if (documentNode === 'pre-advice') return <PreAdviceDocument ownerAccount={owner} preAdviceId={lineKey} panel={panel} />;
  if (documentNode === 'counts') return <CountDocument countId={lineKey} panel={panel} />;
  if (documentNode === 'adjustments') return <AdjustmentDocument adjustmentId={lineKey} panel={panel} />;
  if (documentNode === 'freezes') return <FreezeDocument freezeId={lineKey} panel={panel} />;
  return (
    <StockDetail kind={headers[0] ?? '这一行'} code={line?.cells[0] ?? lineKey} onClose={onClose}>
      <WmsInfoGrid
        items={headers.map((header, index) => ({
          label: header,
          value: header === '状态' ? labelOf(line?.cells[index]) : (line?.cells[index] ?? ''),
        }))}
      />
    </StockDetail>
  );
}

function monitorCell(
  header: string,
  value: string,
  line: MonitorLine,
  index: number,
  ownerNames: ReadonlyMap<string, string>,
  warehouseNames: ReadonlyMap<string, string>,
) {
  if (index === 0 && line.hash) {
    const link = linkOf(line.hash);
    if (link) {
      return (
        <DocumentLink moduleId={link.moduleId} parts={link.parts}>
          {value}
        </DocumentLink>
      );
    }
  }
  if (header === '状态') return <WmsBadge code={value} />;
  if (header === '货主' || header === '仓') {
    const name = nameOf(header === '货主' ? ownerNames : warehouseNames, value);
    return name ? <WmsTwoLine primary={name} secondary={value} code title={`${name} · ${value}`} /> : <span className="whitespace-nowrap font-mono">{value}</span>;
  }
  if (numericHeaders.has(header)) return <span className="whitespace-nowrap tabular-nums">{value}</span>;
  return <span className="whitespace-nowrap">{labelOf(value, value)}</span>;
}

/** 列表行上的地址是 `#/模块/单据键`。点单号进整页，点行在右边开，两处用同一份键。 */
function linkOf(hash: string): { moduleId: string; parts: string[] } | null {
  if (!hash.startsWith('#/')) return null;
  const rest = hash.slice(2);
  const slash = rest.indexOf('/');
  if (slash < 0) return null;
  return { moduleId: rest.slice(0, slash), parts: documentKeyParts(decodeURIComponent(rest.slice(slash + 1))) };
}

function monitorTakesPageSize(node: MonitorNode): boolean {
  return !node.unavailable && node.id !== 'cells' && node.id !== 'cartons';
}

function monitorIsSheet(node: MonitorNode): boolean {
  return node.id === 'cells' || node.id === 'cartons';
}

async function readView(node: MonitorNode, warehouse: string, owner: string, status: string, after: string | null, pageSize: number, sheetPage: number): Promise<View> {
  if (node.unavailable) return { title: node.label, headers: [], lines: [], note: node.unavailable, next: null };
  if (node.needsOwner && owner.trim() === '') {
    return { title: node.label, headers: [], lines: [], note: '这个对象按货主取数。先选一位货主，再看。', next: null };
  }
  const loaded = await loadLines(node, warehouse, owner, status, after, pageSize, sheetPage);
  return { title: node.label, ...loaded };
}

async function loadLines(node: MonitorNode, warehouse: string, owner: string, status: string, after: string | null, pageSize: number, sheetPage: number): Promise<Omit<View, 'title'>> {
  const conditions = monitorTakesPageSize(node)
    ? { filters: monitorFilters(node, warehouse, owner, status), pageSize }
    : { filters: monitorFilters(node, warehouse, owner, status) };
  if (node.id === 'outbound') {
    const answer = await listOutboundRequests(conditions, after);
    if (answer.kind !== 'outcome') return { headers: outboundHeaders, lines: [], note: '出库单没读成。', next: null };
    return { headers: outboundHeaders, lines: outboundLines(answer.body.requests ?? []), note: '点一行在右边看这张单。要看拣货任务，点「拣货任务」。', next: answer.body.page?.next ?? null, total: answer.body.page?.total ?? null };
  }
  if (node.id === 'waves') {
    const answer = await listWaves(conditions, after);
    if (answer.kind !== 'outcome') return { headers: waveHeaders, lines: [], note: '波次没读成。', next: null };
    return { headers: waveHeaders, lines: waveLines(answer.body.waves ?? []), note: '点一行在右边看这个波次。点「拣货任务」看这一波的每一格。', next: answer.body.page?.next ?? null, total: answer.body.page?.total ?? null };
  }
  if (node.id === 'unclaimed') {
    const answer = await listUnclaimedArrivals(conditions, after);
    if (answer.kind !== 'outcome') return { headers: [], lines: [], note: '待认领没读成。', next: null };
    return {
      headers: ['到货', '仓', '状态'],
      lines: (answer.body.arrivals ?? []).map((row) => ({ key: row.arrivalId, cells: [row.arrivalId, row.warehouseCode, row.status], hash: documentHash('warehouse-unclaimed', [row.arrivalId]) })),
      note: '',
      next: answer.body.page?.next ?? null,
      total: answer.body.page?.total ?? null,
    };
  }
  if (node.id === 'pre-advice') {
    const answer = await listOwnerPreAdvices(owner.trim(), '', after, pageSize, '', warehouse.trim());
    if (answer.kind !== 'outcome') return { headers: [], lines: [], note: '入库预报没读成。', next: null };
    return {
      headers: ['预报', '仓', '状态'],
      lines: (answer.body.preAdvices ?? []).map((row) => ({ key: row.preAdviceId, cells: [row.documentNumber ?? row.preAdviceId, row.warehouseCode, row.status], hash: documentHash('warehouse-pre-advices', [owner.trim(), row.preAdviceId]) })),
      note: '',
      next: answer.body.page?.next ?? null,
      total: answer.body.page?.total ?? null,
    };
  }
  if (node.id === 'cells' || node.id === 'cartons') {
    const answer = await queryOwnerStock(owner.trim(), warehouse.trim(), node.id === 'cells' ? 'cells' : 'cartons', null, pageSize, sheetPage);
    if (answer.kind !== 'outcome') return { headers: [], lines: [], note: '库存没读成。', next: null };
    if (node.id === 'cells') {
      return {
        headers: ['库位', '商品', '状态', '在库'],
        lines: (answer.body.cells ?? []).map((cell) => ({ key: `${cell.location}|${cell.goodsCode}|${cell.inboundBatchId}`, cells: [cell.location ?? '', cell.goodsCode, cell.stockState, String(cell.quantity)] })),
        note: '这一位货主的库存格。仓选了就只问这个仓。',
        next: null,
        book: answer.body.book ?? null,
      };
    }
    return {
      headers: ['箱号', '库位'],
      lines: (answer.body.cartons ?? []).map((carton) => ({ key: carton.cartonNumber, cells: [carton.cartonNumber, carton.location ?? ''] })),
      note: '这一位货主的整箱。仓选了就只问这个仓。',
      next: null,
      book: answer.body.book ?? null,
    };
  }
  if (node.id === 'counts') {
    const answer = await listCounts(conditions, after);
    if (answer.kind !== 'outcome') return { headers: [], lines: [], note: '盘点没读成。', next: null };
    return {
      headers: ['盘点单', '仓', '状态', '库位'],
      lines: (answer.body.counts ?? []).map((row) => ({ key: row.countId, cells: [row.countId, row.warehouseCode, row.status, String(row.locations.length)], hash: documentHash(countsAdjustmentsModule, [countKind, row.countId]) })),
      note: '',
      next: answer.body.page?.next ?? null,
      total: answer.body.page?.total ?? null,
    };
  }
  if (node.id === 'adjustments') {
    const answer = await listAdjustments(conditions, after);
    if (answer.kind !== 'outcome') return { headers: [], lines: [], note: '调整没读成。', next: null };
    return {
      headers: ['调整', '货主', '仓', '状态'],
      lines: (answer.body.adjustments ?? []).map((row) => ({ key: row.adjustmentId, cells: [row.adjustmentId, row.ownerAccount, row.warehouseCode, row.status], hash: documentHash(countsAdjustmentsModule, [adjustmentKind, row.adjustmentId]) })),
      note: '',
      next: answer.body.page?.next ?? null,
      total: answer.body.page?.total ?? null,
    };
  }
  if (node.id === 'freezes') {
    const answer = await listFreezes(conditions, after);
    if (answer.kind !== 'outcome') return { headers: [], lines: [], note: '冻结没读成。', next: null };
    return {
      headers: ['冻结', '货主', '仓', '状态'],
      lines: (answer.body.freezes ?? []).map((row) => ({ key: row.freezeId, cells: [row.freezeId, row.ownerAccount, row.warehouseCode, row.status], hash: documentHash(processingModule, [freezeKind, row.freezeId]) })),
      note: '',
      next: answer.body.page?.next ?? null,
      total: answer.body.page?.total ?? null,
    };
  }
  return { headers: [], lines: [], note: '', next: null };
}
