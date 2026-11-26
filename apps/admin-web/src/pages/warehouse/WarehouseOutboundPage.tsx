import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Plus, Send } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import { retainSelectedRows } from '../../templates/list-selection';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { enterStation } from '../../shell/enter-station';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { listOutboundRequests, type OutboundRequestRecord } from './api';
import { DocumentLink } from './document-page';
import { fitColumnWidth, linesSummary, nameOf, shortInstant, waitedSortParam } from './list-cells';
import { hiddenInBasket, mergeWaveBlocker, refreshedBasket, waveReleaseTookEffect } from './outbound-basket';
import { OutboundDocument } from './OutboundDocument';
import { outboundNextStep, outboundSettled } from './outbound-next-step';
import {
  allQueues,
  attentionQueue,
  countedStatuses,
  isWorkingQueue,
  knownQueue,
  queueCount,
  queueStatusesOf,
  settledQueues,
  workingQueues,
} from './outbound-queues';
import { useDocumentParts } from './use-document';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const moduleId = 'warehouse-outbound';
const info = moduleInfoById[moduleId];

/** 一队在页上的叫法：待处理是两种卡住状态合起来的一队，其余各队就叫它那个状态的名字。 */
const queueLabel = (queue: string) => (queue === attentionQueue ? '待处理' : labelOf(queue));
const dispatchWords = ['PRODUCT_NETWORK', 'OWNER_LABEL', 'PICKUP'] as const;
/** 件型筛选里「单品单件」的值；读口的筛选维是 singleUnit。 */
const singleUnitProfile = 'SINGLE_UNIT';

// 「已等」是唯一能排的列（读口只按受理时刻排）。等得久的在前、刚受理的在前，各是它的降序与升序。
const waitedKey = 'waited';
const longestWaitFirst: WorkbenchSort = { key: waitedKey, dir: -1 };
const newestFirst: WorkbenchSort = { key: waitedKey, dir: 1 };

/** 卡在预占上、要人拿主意的两种状态：这样的行在列表里带底色。 */
const stuck = (row: OutboundRequestRecord) => row.status === 'PARTIALLY_RESERVED' || row.status === 'PENDING_POLICY';

const rowKeyOf = (row: OutboundRequestRecord) => row.requestId;
// 没有行时交同一个空数组：每次渲染交一个新的，照着它做的副作用就每次渲染都跑。
const noRows: OutboundRequestRecord[] = [];

/** 按内容定宽的列里，一格的内容不折行：列宽是照它算的，折了行列就被挤窄。 */
const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/** 出库单：列表与每一张出库单各占自己的标签，同一个模块按地址里有没有单据键分。 */
export function WarehouseOutboundPage() {
  const parts = useDocumentParts();
  return parts ? <OutboundDocument requestId={parts[0]} /> : <OutboundList />;
}

/**
 * 出库单列表，摆在单据工作台上：标题行上是各队还有几张、点了就看那一队，下面一行工具条（检索与筛选），点一行在右边开这张单——
 * 看它、做它的动作都在栏里，不必另开标签；单号仍是链接，点了开整页。勾选是另一件事：圈几张同仓、已预占的单合并放波次。
 * 翻页条在页底。
 *
 * 各队的数量不摆在工具条里：十个带数的签平铺开要占两行，而它们说的是「哪一队积压了多少」，是这一页的概况，
 * 标题行右边本来空着。按哪一队筛在工具条的状态下拉里也能选，两处是同一个筛选。
 *
 * 检索词、选的队、筛选、排序与看的是哪一行记在标签的视图状态上：切到别的标签再回来还在。
 *
 * 勾选是一篮，不跟着眼前显示的内容走（outbound-basket.ts 文件头）：切队、检索、换筛选、翻页都留着，只在点「取消选择」
 * 或合并放波次放成了之后清。批量栏只要篮里有单就在，说清有几张不在眼前这一页；「只看已选」把篮里的单摆出来核对。
 * 篮子不记进标签的视图状态：它是一次批量动作的暂存，页面重挂之后行可能已经变了。
 */
function OutboundList() {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const [keyword, setKeyword] = useTabViewState('warehouse-outbound.keyword', '');
  const [storedQueue, setQueue] = useTabViewState<string>('warehouse-outbound.status', allQueues);
  const queue = knownQueue(storedQueue);
  const [owner, setOwner] = useTabViewState('warehouse-outbound.owner', '');
  const [warehouse, setWarehouse] = useTabViewState('warehouse-outbound.warehouse', '');
  const [dispatch, setDispatch] = useTabViewState('warehouse-outbound.dispatch', '');
  // 件型：现在只有「单品单件」一种可筛——整张单要拣的只有一件。筛出来放成一个波次，拣货时同库位同商品合成一格拿。
  const [profile, setProfile] = useTabViewState('warehouse-outbound.profile', '');
  const [chosenSort, setChosenSort] = useTabViewState<WorkbenchSort | null>('warehouse-outbound.sort', null);
  const [openKey, setOpenKey] = useTabViewState<string | null>('warehouse-outbound.open', null);
  const [basket, setBasket] = useState<ReadonlyMap<string, OutboundRequestRecord>>(() => new Map());
  const [onlyBasket, setOnlyBasket] = useState(false);
  // 开着的放波次表单放的是不是篮里的这一批：行上的「放波次」也开同一张表单，那一种放成了不该清篮子。
  const mergingBasket = useRef(false);
  const [refreshes, setRefreshes] = useState(0);

  const settled = useSettledKeyword(keyword);
  const baseFilters: Record<string, string[]> = {};
  if (owner) baseFilters.ownerAccount = [owner];
  if (warehouse) baseFilters.warehouseCode = [warehouse];
  if (dispatch) baseFilters.dispatch = [dispatch];
  if (profile === singleUnitProfile) baseFilters.singleUnit = ['true'];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  // 选了某一队之后等得最久的在最上面；看全部或已了结的，最新受理的在上面。人点过表头就照他的。
  const sort = chosenSort ?? (isWorkingQueue(queue) ? longestWaitFirst : newestFirst);
  const conditions: CatalogueConditions = {
    ...base,
    sort: waitedSortParam(sort.dir),
    filters: queue === allQueues ? baseFilters : { ...baseFilters, status: queueStatusesOf(queue) },
  };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listOutboundRequests);
  // 重取在途时垫着上一页的行：不然每做完一个动作，列表退回骨架、详情栏跟着收起再打开，栏里刚做完的那张单要重读一遍。
  // 垫着的只是行，各队的数量仍只认各自那一问。
  const held = heldCataloguePage(answer, previous, (body) => body.requests.length > 0);
  const pageRows = answer?.kind === 'outcome' ? answer.body.requests : (held?.body.requests ?? noRows);
  useEffect(() => {
    setBasket((current) => refreshedBasket(current, pageRows, rowKeyOf));
  }, [pageRows]);
  // 篮子空了「只看已选」就没有可看的，回到列表。
  const showingBasket = onlyBasket && basket.size > 0;
  const basketRecords = useMemo(() => [...basket.values()], [basket]);
  const checkedKeys = useMemo(() => new Set(basket.keys()), [basket]);
  const rows = showingBasket ? basketRecords : pageRows;
  const counts = useStatusCounts(listOutboundRequests, base, 'status', countedStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };

  const agentWarehouses = useMemo(
    () => new Set(drawer.lookups.warehouses.filter((w) => w.kind === 'AGENT').map((w) => w.warehouseCode)),
    [drawer.lookups.warehouses],
  );
  const ownerNames = useMemo(
    () => new Map(drawer.lookups.owners.map((account) => [account.accountId, account.customerPartyName ?? ''])),
    [drawer.lookups.owners],
  );
  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((w) => [w.warehouseCode, w.name ?? ''])),
    [drawer.lookups.warehouses],
  );
  const now = new Date();

  // 勾上的行连同它的内容记进篮子：翻走之后页面手里没有它了，而能不能合并放波次要照行来判。
  const changeChecked = (next: Set<string>) => setBasket((current) => retainSelectedRows(current, next, rows, rowKeyOf));
  const hiddenChecked = hiddenInBasket(basket, pageRows, rowKeyOf);
  const mergeBlocker = mergeWaveBlocker(basketRecords);
  // 换了队、检索词、筛选或排序，是要看那一份列表：正在「只看已选」的话先回到列表。篮子不动。
  const narrowing =
    <Value,>(apply: (value: Value) => void) =>
    (value: Value) => {
      apply(value);
      setOnlyBasket(false);
    };

  // 人认名称，编码放在下面那一行；名册里没有名称的只摆编码。
  const named = (names: ReadonlyMap<string, string>, code: string) => {
    const name = nameOf(names, code);
    return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
  };

  // 列表变窄时（开了详情栏、或窗口小）从次要的列收起：先收商品、发运、仓库、货主，收件人最后；单号、状态、件数、
  // 已等与下一步一直在。各个门槛是这一列出现时整张表刚好摆得下的列表宽度。
  const columns: WorkbenchColumn<OutboundRequestRecord>[] = [
    {
      key: 'id',
      header: '出库单',
      width: fitColumnWidth,
      render: (row) => (
        <WmsTwoLine
          primary={
            <DocumentLink moduleId={moduleId} parts={[row.requestId]}>
              {row.documentNumber ?? row.requestId}
            </DocumentLink>
          }
          secondary={`${labelOf(row.origin)} · ${shortInstant(row.receivedAt, now)}`}
          title={`受理于 ${formatInstant(row.receivedAt)}`}
        />
      ),
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} /> },
    { key: 'owner', header: '货主', width: fitColumnWidth, minListWidth: 880, render: (row) => named(ownerNames, row.ownerAccount) },
    { key: 'warehouse', header: '仓库', width: fitColumnWidth, minListWidth: 1060, render: (row) => named(warehouseNames, row.warehouseCode) },
    { key: 'dispatch', header: '发运', width: fitColumnWidth, minListWidth: 1150, render: (row) => nowrap(labelOf(row.dispatch)) },
    {
      key: 'recipient',
      header: '收件人',
      minListWidth: 700,
      render: (row) => <WmsTwoLine fill primary={row.recipient} secondary={row.address} title={`${row.recipient}\n${row.address}`} />,
    },
    {
      key: 'lines',
      header: '商品',
      minListWidth: 1340,
      render: (row) => {
        const summary = linesSummary(row.lines);
        if (summary.first === '') return <span className="whitespace-nowrap text-idpxyz-textMuted">没有商品行</span>;
        return <WmsTwoLine fill primary={<span className="font-mono">{summary.first}</span>} secondary={summary.rest} title={summary.all} />;
      },
    },
    {
      key: 'qty',
      header: nowrap('件数'),
      right: true,
      width: fitColumnWidth,
      // 只在卡在预占的两种状态上说占到多少：拣出去的那部分预占已经放开，拣完之后的 0 不是缺货。
      render: (row) => {
        const wanted = row.lines.reduce((sum, line) => sum + line.quantity, 0);
        return (
          <div className="whitespace-nowrap leading-tight">
            <div className="tabular-nums">{wanted}</div>
            {stuck(row) ? <div className="mt-0.5 type-caption text-[color:var(--ds-status-warning-fg)]">只占到 {row.reservedQuantity}</div> : null}
          </div>
        );
      },
    },
    {
      key: 'age',
      header: nowrap('已等'),
      right: true,
      width: fitColumnWidth,
      sortKey: waitedKey,
      // 已了结的单不写等了多久：它不在等谁。受理的时刻在单号下面那一行。
      render: (row) =>
        outboundSettled(row.status) ? (
          <span className="text-idpxyz-textMuted">—</span>
        ) : (
          <span className="whitespace-nowrap tabular-nums">{elapsedLabel(row.receivedAt, now) || '—'}</span>
        ),
    },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) => <NextStepCell row={row} agentWarehouse={agentWarehouses.has(row.warehouseCode)} drawer={drawer} />,
    },
  ];

  const chooseQueue = narrowing(setQueue);
  // 标题行上的各队：数量是当前检索与筛选下这一队还有几张。点一下看这一队，再点一下回到全部。
  const queueStats: WmsQueueStat[] = workingQueues.map((id) => ({
    id,
    label: queueLabel(id),
    count: queueCount(counts, id),
    active: queue === id,
    attention: id === attentionQueue,
    onClick: () => chooseQueue(queue === id ? allQueues : id),
  }));

  const narrowed =
    settled.trim() !== '' || queue !== allQueues || owner !== '' || warehouse !== '' || dispatch !== '' || profile !== '';
  const clearFilters = () => {
    setKeyword('');
    setQueue(allQueues);
    setOwner('');
    setWarehouse('');
    setDispatch('');
    setProfile('');
    setOnlyBasket(false);
  };
  // 只看已选时摆的是篮里的行，读口这一问答成什么样都不改它：照就绪摆。
  const viewState: TemplateViewState = held || showingBasket
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-outbound-request-list',
        emptyTitle: '还没有出库单',
        emptyDescription: '点右上角「代录出库单」新建，或等货主经客户接口提交。',
        narrowed: narrowed || pastFirstPage(pagination),
      });
  // 垫着上一页时翻页条照那一页写，两个钮按了不动：拿上一页的游标去翻，翻的是一组可能已经换掉的条件。
  // 只看已选时摆的不是读口的某一页，没有页可翻。
  const shownPagination = showingBasket ? undefined : (pagination ?? held?.pagination);

  return (
    <>
      <WorkbenchPageTemplate<OutboundRequestRecord>
        title={info.title}
        viewStateKey={moduleId}
        subtitle="点一行在右边看这张单、做它的下一步"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          drawer.has('outbound-request') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('outbound-request')}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              代录出库单
            </Button>
          ) : undefined
        }
        search={{ value: keyword, onChange: narrowing(setKeyword), placeholder: '搜出库单号、货主、仓、收件人…' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={queue === allQueues ? '' : queue}
              onChange={(next) => chooseQueue(next === '' ? allQueues : next)}
              options={[...workingQueues, ...settledQueues].map((id) => ({ value: id, label: queueLabel(id) }))}
            />
            <WmsFilterSelect
              label="货主"
              value={owner}
              onChange={narrowing(setOwner)}
              options={drawer.lookups.owners.map((account) => ({
                value: account.accountId,
                label: account.customerPartyName ? `${account.customerPartyName} · ${account.accountId}` : account.accountId,
              }))}
            />
            <WmsFilterSelect
              label="仓库"
              value={warehouse}
              onChange={narrowing(setWarehouse)}
              options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: w.name ? `${w.name} · ${w.warehouseCode}` : w.warehouseCode }))}
            />
            <WmsFilterSelect
              label="发运"
              value={dispatch}
              onChange={narrowing(setDispatch)}
              options={dispatchWords.map((word) => ({ value: word, label: labelOf(word) }))}
            />
            <WmsFilterSelect
              label="件型"
              value={profile}
              onChange={narrowing(setProfile)}
              options={[{ value: singleUnitProfile, label: '单品单件' }]}
            />
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
        rows={rows}
        rowKey={(row) => row.requestId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        selection={{ selected: checkedKeys, onChange: changeChecked }}
        bulkActions={
          <>
            {hiddenChecked > 0 && !showingBasket ? (
              <span className="whitespace-nowrap type-body text-idpxyz-textMuted">其中 {hiddenChecked} 张不在当前列表</span>
            ) : null}
            <Button type="button" size="sm" variant="outline" aria-pressed={showingBasket} onClick={() => setOnlyBasket(!showingBasket)}>
              {showingBasket ? '回到列表' : '只看已选'}
            </Button>
            {drawer.has('wave-release') ? (
              <>
                <Button
                  type="button"
                  size="sm"
                  className={primaryActionClass}
                  disabled={mergeBlocker !== null}
                  title={mergeBlocker ?? '把勾选的出库单合成一个波次放出去'}
                  onClick={() => {
                    mergingBasket.current = true;
                    drawer.open('wave-release', {
                      warehouse: basketRecords[0]?.warehouseCode ?? '',
                      requests: basketRecords.map(rowKeyOf).join(', '),
                    });
                  }}
                >
                  <Send className="mr-1 h-3.5 w-3.5" />
                  {`合并放波次（${basketRecords.length} 张）`}
                </Button>
                {/* 钮按不动时把原因写在旁边：悬停才看得到的话，人只知道按不动、不知道该去掉哪一张。 */}
                {mergeBlocker ? <span className="type-caption text-idpxyz-textMuted">{mergeBlocker}</span> : null}
              </>
            ) : null}
          </>
        }
        rowAttention={stuck}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => narrowing(setChosenSort)(toggleWorkbenchSort(sort, key))}
        noMatchText="当前筛选下没有出库单"
        onClearFilters={clearFilters}
        renderDetail={(key) => (
          // 换一行就是换一张单：按键重挂，上一张单读到一半的答复与没关的表单不带过来。
          <OutboundDocument key={key} requestId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />
        )}
        viewState={viewState}
      />
      {shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : null}
      <ActionDrawer
        moduleId={moduleId}
        active={drawer.active}
        lookups={drawer.lookups}
        onClose={() => {
          mergingBasket.current = false;
          drawer.close();
        }}
        onSubmitted={(outcome) => {
          // 只在篮里的这一批放成了才清篮子：别的动作、或没放成的答复，勾着的单留着，改完不必重新勾一遍。
          if (mergingBasket.current && waveReleaseTookEffect(outcome)) {
            setBasket(new Map());
            setOnlyBasket(false);
          }
          refresh();
        }}
      />
    </>
  );
}

/** 每行末尾的那一格：这一单此刻该做的一步。能做的是一个钮，等人定的只说在等什么。 */
function NextStepCell({
  row,
  agentWarehouse,
  drawer,
}: {
  row: OutboundRequestRecord;
  agentWarehouse: boolean;
  drawer: ActionDrawerControl;
}) {
  const next = outboundNextStep(row.status, agentWarehouse, undefined, row.requestId);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (next.kind === 'wait') return <span className="whitespace-nowrap type-caption text-[color:var(--ds-status-warning-fg)]">{next.label}</span>;
  if (next.kind === 'wave' && !drawer.has('wave-release')) return <span className="whitespace-nowrap text-idpxyz-textMuted">等放波次</span>;
  // 能做的一步在每一行上是同一个样子的钮，不论它是就地开表单还是去别的页：操作者要认的是「这一单现在该做什么」，
  // 不是这一步落在哪里。
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      // 字用链接色：表里除了状态徽章只有能点的东西带颜色，灰描边灰字的钮在一排灰字里找不出来。
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        // 行上的单击是开详情：这一下只做这一步，不顺带把详情栏开合一次。
        event.stopPropagation();
        if (next.kind === 'wave') drawer.open('wave-release', { warehouse: row.warehouseCode, requests: row.requestId });
        else if (next.kind === 'station') enterStation(next.hash);
        else window.location.hash = `#/${next.moduleId}`;
      }}
    >
      {next.label}
    </Button>
  );
}
