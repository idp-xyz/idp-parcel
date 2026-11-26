import { useEffect, useRef, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { Button, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import {
  getOutboundRequestDetail,
  listAdjustments,
  listCounts,
  listFreezes,
  listOutboundRequests,
  listOwnerPreAdvices,
  listUnclaimedArrivals,
  listWaves,
  queryOwnerStock,
} from './api';
import { documentHash } from './document-address';
import { adjustmentKind, countKind, countsAdjustmentsModule } from './counts-adjustments-address';
import { freezeKind, processingModule } from './processing-address';
import { labelOf, WmsBadge, WmsFilterSelect, WmsPagerBar } from './wms-ui';
import { useWarehouseLookups } from './use-catalogues';
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
  type MonitorLine,
  type MonitorNode,
} from './monitor-tree';

const info = moduleInfoById['warehouse-monitor'];
const opening = monitorNodes.find((item) => item.id === 'outbound') ?? monitorNodes[0];

interface View {
  title: string;
  headers: string[];
  lines: MonitorLine[];
  note: string;
  next: string | null;
}

export function WarehouseMonitorPage() {
  const lookups = useWarehouseLookups();
  const [node, setNode] = useState<MonitorNode>(opening);
  const [warehouse, setWarehouse] = useState('');
  const [owner, setOwner] = useState('');
  const [status, setStatus] = useState('');
  const [nonce, setNonce] = useState(0);
  const [cursor, setCursor] = useState<MonitorCursor>(monitorCursorStart);
  const [stack, setStack] = useState<View[]>([]);
  const [view, setView] = useState<View>({ title: opening.label, headers: [], lines: [], note: '', next: null });
  const [loading, setLoading] = useState(true);
  const generation = useRef(0);

  useEffect(() => {
    setLoading(true);
    setStack([]);
    setView({ title: node.label, headers: [], lines: [], note: '', next: null });
    const timer = setTimeout(() => {
      const mine = ++generation.current;
      void (async () => {
        const next = await readView(node, warehouse, owner, status, cursor.after);
        if (mine !== generation.current) return;
        setView(next);
        setLoading(false);
      })();
    }, 250);
    return () => clearTimeout(timer);
  }, [node, warehouse, owner, status, nonce, cursor.after]);

  const drill = async (line: MonitorLine) => {
    if (line.drillTasks) {
      setStack((current) => [...current, view]);
      setView({ title: `${line.cells[0]} 的拣货任务`, headers: taskHeaders, lines: line.drillTasks, note: '波次上的任务没有货主，这一层不再钻库存格。要从出库单钻下去。', next: null });
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
    if (stock) {
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
    }
  };

  const choose = (item: MonitorNode) => {
    setNode(item);
    setStatus('');
    setCursor(monitorCursorStart());
  };

  const narrow = (apply: () => void) => {
    apply();
    setCursor(monitorCursorStart());
  };

  return (
    <div className="flex min-h-0 flex-1 bg-idpxyz-editor">
      <aside aria-label="对象树" className="w-44 shrink-0 overflow-y-auto border-r border-idpxyz-border bg-idpxyz-panel p-3">
        {monitorGroups().map((group) => (
          <div key={group.group} className="mb-3">
            <div className="type-caption font-semibold text-idpxyz-textBright">{group.group}</div>
            {group.nodes.map((item) => (
              <button key={item.id} type="button" className={`mt-1 block w-full rounded px-2 py-1 text-left type-body ${item.id === node.id ? 'bg-idpxyz-accent/15 text-idpxyz-textBright' : 'text-idpxyz-textMuted hover:bg-idpxyz-hover'}`} onClick={() => choose(item)}>
                {item.label}
              </button>
            ))}
          </div>
        ))}
      </aside>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <div className="flex flex-wrap items-center gap-2 border-b border-idpxyz-border px-4 py-3">
          <div className="mr-auto">
            <h1 className="type-title font-bold text-idpxyz-textBright">{info?.title ?? '仓库监控'}</h1>
            <p className="type-caption text-idpxyz-textMuted">{stack.map((item) => item.title).concat(view.title).join(' / ')}</p>
          </div>
          {stack.length > 0 ? <Button type="button" variant="ghost" size="sm" onClick={() => { const previous = stack[stack.length - 1]; setStack(stack.slice(0, -1)); setView(previous); }}>返回上一层</Button> : null}
          {(node.warehouse || node.id === 'pre-advice') ? (
            <WmsFilterSelect
              label="仓"
              value={warehouse}
              options={lookups.warehouses.map((item) => ({ value: item.warehouseCode, label: item.name ? `${item.name}` : item.warehouseCode }))}
              onChange={(next) => narrow(() => setWarehouse(next))}
            />
          ) : null}
          {(node.owner || node.needsOwner) ? (
            <WmsFilterSelect
              label="货主"
              value={owner}
              options={lookups.owners.map((item) => ({ value: item.accountId, label: item.customerPartyName ? `${item.customerPartyName}` : item.accountId }))}
              onChange={(next) => narrow(() => setOwner(next))}
            />
          ) : null}
          {node.status ? (
            <WmsFilterSelect
              label="状态"
              value={status}
              options={node.status.map((code) => ({ value: code, label: labelOf(code, code) }))}
              onChange={(next) => narrow(() => setStatus(next))}
            />
          ) : null}
          <Button type="button" variant="outline" size="sm" disabled={loading} onClick={() => setNonce((current) => current + 1)}>
            <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
            {loading ? '刷新中…' : '刷新'}
          </Button>
        </div>
        <div className="min-h-0 flex-1 overflow-auto p-4">
          {view.note ? <p className="type-body text-idpxyz-textMuted">{view.note}</p> : null}
          {loading && view.lines.length === 0 && !view.note ? <p className="type-body text-idpxyz-textMuted">在读…</p> : null}
          {view.headers.length > 0 ? (
            <div className="mt-3 overflow-hidden rounded-lg border border-idpxyz-border bg-idpxyz-panel">
              <Table>
                <TableHeader>
                  <TableRow>
                    {view.headers.map((header) => <TableHead key={header}>{header}</TableHead>)}
                    {view.lines.some((line) => line.drillLabel) ? <TableHead className="text-right">往下</TableHead> : null}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {view.lines.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={view.headers.length} className="py-8 text-center text-idpxyz-textMuted">这一页没有。</TableCell>
                    </TableRow>
                  ) : view.lines.map((line) => (
                    <TableRow key={line.key}>
                      {line.cells.map((cell, index) => (
                        <TableCell key={view.headers[index]} className={monitorCellClass(view.headers[index])}>
                          {index === 0 && line.hash ? <a className="text-idpxyz-accent hover:underline" href={line.hash}>{labelOf(cell, cell)}</a> : null}
                          {index === 0 && !line.hash ? labelOf(cell, cell) : null}
                          {index > 0 && view.headers[index] === '状态' ? <WmsBadge code={cell} /> : null}
                          {index > 0 && view.headers[index] !== '状态' ? labelOf(cell, cell) : null}
                        </TableCell>
                      ))}
                      {view.lines.some((item) => item.drillLabel) ? (
                        <TableCell className="text-right">
                          {line.drillLabel ? <Button type="button" variant="ghost" size="sm" onClick={() => void drill(line)}>{line.drillLabel}</Button> : null}
                        </TableCell>
                      ) : null}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          ) : null}
        </div>
        {/* 钻下去的那一层是从上一层带出来的几行，不翻页；回到顶层翻页条才出来。在读时它留在原处、只把钮按住，
            不然每翻一页它都要消失再出现一次。 */}
        {stack.length === 0 ? (
          <WmsPagerBar
            summary={`第 ${cursor.trail.length} 页`}
            canPrevious={!loading && monitorCursorCanPrevious(cursor)}
            canNext={!loading && view.next !== null}
            onPrevious={() => setCursor(monitorCursorPrevious(cursor))}
            onNext={() => { const next = view.next; if (next !== null) setCursor(monitorCursorNext(cursor, next)); }}
          />
        ) : null}
      </div>
    </div>
  );
}

const monoHeaders = new Set(['出库单', '货主', '仓', '到货', '预报', '波次', '任务', '库位', '商品', '箱号', '盘点单', '调整', '冻结']);

function monitorCellClass(header: string): string {
  return monoHeaders.has(header) ? 'whitespace-nowrap font-mono' : 'whitespace-nowrap';
}

async function readView(node: MonitorNode, warehouse: string, owner: string, status: string, after: string | null): Promise<View> {
  if (node.unavailable) return { title: node.label, headers: [], lines: [], note: node.unavailable, next: null };
  if (node.needsOwner && owner.trim() === '') {
    return { title: node.label, headers: [], lines: [], note: '这个对象按货主取数。先填货主，再看。不替你选一个货主。', next: null };
  }
  const loaded = await loadLines(node, warehouse, owner, status, after);
  return { title: node.label, ...loaded };
}

async function loadLines(node: MonitorNode, warehouse: string, owner: string, status: string, after: string | null): Promise<Omit<View, 'title'>> {
  const conditions = { filters: monitorFilters(node, warehouse, owner, status) };
  if (node.id === 'outbound') {
    const answer = await listOutboundRequests(conditions, after);
    if (answer.kind !== 'outcome') return { headers: outboundHeaders, lines: [], note: '出库单没读成。', next: null };
    return { headers: outboundHeaders, lines: outboundLines(answer.body.requests ?? []), note: '点单号打开这张单。要看拣货任务，点这一行的「拣货任务」。', next: answer.body.page?.next ?? null };
  }
  if (node.id === 'waves') {
    const answer = await listWaves(conditions, after);
    if (answer.kind !== 'outcome') return { headers: waveHeaders, lines: [], note: '波次没读成。', next: null };
    return { headers: waveHeaders, lines: waveLines(answer.body.waves ?? []), note: '点波次号打开波次。点「拣货任务」看这一波的每一格。', next: answer.body.page?.next ?? null };
  }
  if (node.id === 'unclaimed') {
    const answer = await listUnclaimedArrivals(conditions, after);
    if (answer.kind !== 'outcome') return { headers: [], lines: [], note: '待认领没读成。', next: null };
    return {
      headers: ['到货', '仓', '状态'],
      lines: (answer.body.arrivals ?? []).map((row) => ({ key: row.arrivalId, cells: [row.arrivalId, row.warehouseCode, row.status], hash: documentHash('warehouse-unclaimed', [row.arrivalId]) })),
      note: '',
      next: answer.body.page?.next ?? null,
    };
  }
  if (node.id === 'pre-advice') {
    const answer = await listOwnerPreAdvices(owner.trim(), '', after);
    if (answer.kind !== 'outcome') return { headers: [], lines: [], note: '入库预报没读成。', next: null };
    const warehouseCode = warehouse.trim();
    const rows = (answer.body.preAdvices ?? []).filter((row) => warehouseCode === '' || row.warehouseCode === warehouseCode);
    return {
      headers: ['预报', '仓', '状态'],
      lines: rows.map((row) => ({ key: row.preAdviceId, cells: [row.documentNumber ?? row.preAdviceId, row.warehouseCode, row.status], hash: documentHash('warehouse-pre-advices', [owner.trim(), row.preAdviceId]) })),
      note: '仓是在这一页上留下的，读口不按仓筛。',
      next: answer.body.page?.next ?? null,
    };
  }
  if (node.id === 'cells' || node.id === 'cartons') {
    const answer = await queryOwnerStock(owner.trim(), warehouse.trim(), node.id === 'cells' ? 'cells' : 'cartons');
    if (answer.kind !== 'outcome') return { headers: [], lines: [], note: '库存没读成。', next: null };
    if (node.id === 'cells') {
      return {
        headers: ['库位', '商品', '状态', '在库'],
        lines: (answer.body.cells ?? []).map((cell) => ({ key: `${cell.location}|${cell.goodsCode}|${cell.inboundBatchId}`, cells: [cell.location ?? '', cell.goodsCode, cell.stockState, String(cell.quantity)] })),
        note: '这一位货主的库存格。仓填了就只问这个仓。这一口不分页。',
        next: null,
      };
    }
    return {
      headers: ['箱号', '库位'],
      lines: (answer.body.cartons ?? []).map((carton) => ({ key: carton.cartonNumber, cells: [carton.cartonNumber, carton.location ?? ''] })),
      note: '这一位货主的整箱。仓填了就只问这个仓。这一口不分页。',
      next: null,
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
    };
  }
  return { headers: [], lines: [], note: '', next: null };
}
