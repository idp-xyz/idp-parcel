import { useMemo, useState, type ReactNode } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, type WorkbenchColumn } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { enterStation } from '../../shell/enter-station';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import type { CatalogueConditions } from '../catalogue-query';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listOwnerPreAdvices, type PreAdviceRecord } from './api';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { PreAdviceDocument } from './PreAdviceDocument';
import { useDocumentParts } from './use-document';
import { receiveStationHash } from './station/stations';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const moduleId = 'warehouse-pre-advices';
const info = moduleInfoById[moduleId];

const allQueues = 'ALL';
const workingStatuses = ['ADVISED', 'RECEIVING'] as const;
const settledStatuses = ['CLOSED', 'CANCELLED'] as const;
const statuses = [...workingStatuses, ...settledStatuses] as const;

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

function loadPreAdvices(conditions: CatalogueConditions, after: string | null) {
  const owner = conditions.selectors?.ownerAccount ?? '';
  if (owner === '') {
    return Promise.resolve({
      kind: 'outcome' as const,
      status: 200,
      body: { outcome: 'PRE_ADVICES_LISTED', ownerAccount: '', preAdvices: [], page: { size: 0, next: null, total: null } },
    });
  }
  return listOwnerPreAdvices(owner, conditions.q ?? '', after);
}

/** 还没开始收、预计到达已经过去的预报算迟到；收货中的已经到了。 */
const lateOf = (row: PreAdviceRecord, now: Date) => (row.status === 'ADVISED' ? elapsedLabel(row.currentNotice?.expectedAt, now) : '');

/** 入库预报：列表与每一张预报各占自己的标签。预报的读口要货主，所以单据键是货主加预报标识两段。 */
export function WarehousePreAdvicesPage() {
  const parts = useDocumentParts();
  return parts ? <PreAdviceDocument ownerAccount={parts[0]} preAdviceId={parts[1] ?? ''} /> : <PreAdviceList />;
}

function PreAdviceList() {
  const drawer = useActionDrawer(receivingActionSpecs);
  const owners = drawer.lookups.owners;
  const [picked, setPicked] = useTabViewState('warehouse-pre-advices.owner', '');
  const [keyword, setKeyword] = useTabViewState('warehouse-pre-advices.keyword', '');
  const [status, setStatus] = useTabViewState('warehouse-pre-advices.status', allQueues);
  const [openKey, setOpenKey] = useTabViewState<string | null>('warehouse-pre-advices.open', null);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(keyword);
  // 预报按货主归属，读口必须带货主：人没挑过、或挑的已经不在名册里，就取名册里的第一家。
  const owner = owners.some((account) => account.accountId === picked) ? picked : (owners[0]?.accountId ?? '');
  const { answer, previous, pagination, retry } = useCatalogueCursor({ selectors: { ownerAccount: owner }, q: settled }, loadPreAdvices);
  const held = heldCataloguePage(answer, previous, (body) => body.preAdvices.length > 0);
  const loaded = answer?.kind === 'outcome' ? answer.body.preAdvices : (held?.body.preAdvices ?? []);
  // 读口不按状态筛，状态是在答回来的这一页里筛的。整份只有一页、并且这一问真是某个货主的，标题上才写数。
  const wholeList = owner !== '' && answer?.kind === 'outcome' && answer.body.page.next === null && (pagination?.page ?? 1) === 1;
  const queue = statuses.includes(status as (typeof statuses)[number]) ? status : allQueues;
  const rows = queue === allQueues ? loaded : loaded.filter((row) => row.status === queue);
  const now = new Date();
  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((warehouse) => [warehouse.warehouseCode, warehouse.name ?? ''])),
    [drawer.lookups.warehouses],
  );
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };

  const namedWarehouse = (code: string) => {
    const name = nameOf(warehouseNames, code);
    return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
  };
  // 开了详情栏之后列表变窄，次要的列收起：单号、状态与下一步一直在。
  const columns: WorkbenchColumn<PreAdviceRecord>[] = [
    {
      key: 'id',
      header: '入库单',
      width: fitColumnWidth,
      render: (row) => (
        <WmsTwoLine
          primary={
            <DocumentLink moduleId={moduleId} parts={[row.ownerAccount, row.preAdviceId]}>
              {row.documentNumber ?? row.preAdviceId}
            </DocumentLink>
          }
          secondary={shortInstant(row.acceptedAt, now)}
          title={`受理于 ${formatInstant(row.acceptedAt)}`}
        />
      ),
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} /> },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 880, render: (row) => namedWarehouse(row.warehouseCode) },
    {
      key: 'mode',
      header: nowrap('到仓 / 入库'),
      width: fitColumnWidth,
      minListWidth: 1080,
      render: (row) => nowrap(`${labelOf(row.arrivalMode)} · ${labelOf(row.inboundMode)}`),
    },
    {
      key: 'size',
      header: nowrap('商品行 / 整箱'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1240,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{`${row.goodsLines?.length ?? 0} / ${row.cartons?.length ?? 0}`}</span>,
    },
    {
      key: 'expected',
      header: nowrap('预计到达'),
      width: fitColumnWidth,
      minListWidth: 1400,
      render: (row) => {
        const expected = row.currentNotice?.expectedAt;
        const late = lateOf(row, now);
        return (
          <span className="inline-flex items-center gap-2 whitespace-nowrap" title={expected ? `预计 ${formatInstant(expected)} 到` : '没有告知预计到达'}>
            <span className="tabular-nums">{expected ? shortInstant(expected, now) : '—'}</span>
            {late ? <WmsBadge label={`已迟 ${late}`} tone="danger" /> : null}
          </span>
        );
      },
    },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) =>
        row.status === 'ADVISED' || row.status === 'RECEIVING' ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
            onClick={(event) => {
              event.stopPropagation();
              enterStation(receiveStationHash(row.preAdviceId));
            }}
          >
            {row.status === 'ADVISED' ? '去收货' : '接着收货'}
          </Button>
        ) : (
          <span className="text-idpxyz-textMuted">—</span>
        ),
    },
  ];

  const chooseQueue = (next: string) => setStatus(next);
  const queueStats: WmsQueueStat[] = workingStatuses.map((code) => ({
    id: code,
    label: labelOf(code),
    count: wholeList ? loaded.filter((row) => row.status === code).length : null,
    active: queue === code,
    attention: code === 'ADVISED' && wholeList && loaded.some((row) => lateOf(row, now) !== ''),
    onClick: () => chooseQueue(queue === code ? allQueues : code),
  }));
  const narrowed = settled.trim() !== '' || queue !== allQueues;
  const clearFilters = () => {
    setKeyword('');
    setStatus(allQueues);
  };
  const viewState: TemplateViewState =
    owner === ''
      ? drawer.lookups.loaded
        ? {
            kind: 'empty',
            title: '还没有货主客户账户',
            description: '入库预报按货主归属。先在国际小包那边的「客户」里登记货主的客户账户。',
          }
        : { kind: 'loading', shape: 'table' }
      : held
        ? { kind: 'ready' }
        : catalogueViewState(answer, loaded.length, refresh, {
            module: info,
            endpoint: 'GET /warehouse-fulfillment-inbound-pre-advices',
            emptyTitle: '这位货主还没有入库预报',
            emptyDescription: '点右上角「代录入库预报」新建，或等货主经客户接口提交。',
            narrowed: narrowed || pastFirstPage(pagination),
          });
  const shownPagination = pagination ?? held?.pagination;
  const noMatchText =
    queue !== allQueues
      ? wholeList
        ? '这位货主名下没有这个状态的入库预报'
        : '这一页里没有这个状态的入库预报。读口不按状态筛，换一页再看。'
      : '当前检索下没有入库预报';

  return (
    <>
      <WorkbenchPageTemplate<PreAdviceRecord>
        title={info.title}
        viewStateKey={moduleId}
        subtitle="点一行在右边看这张预报、做它的下一步"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          drawer.has('pre-advice-on-behalf') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('pre-advice-on-behalf', { ownerAccount: owner })}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              代录入库预报
            </Button>
          ) : undefined
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜预报标识或仓编码' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={queue === allQueues ? '' : queue}
              onChange={(next) => chooseQueue(next === '' ? allQueues : next)}
              options={statuses.map((code) => ({ value: code, label: labelOf(code) }))}
            />
            <label className="inline-flex items-center gap-1.5 type-body text-idpxyz-textMuted">
              <span className="whitespace-nowrap">货主</span>
              <select
                className="h-7 max-w-[16rem] rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text focus:border-idpxyz-accent focus:outline-none"
                value={owner}
                onChange={(event) => setPicked(event.target.value)}
              >
                {owners.length === 0 ? <option value="">没有货主客户账户</option> : null}
                {owners.map((account) => (
                  <option key={account.accountId} value={account.accountId}>
                    {account.customerPartyName ? `${account.customerPartyName} · ${account.accountId}` : account.accountId}
                  </option>
                ))}
              </select>
            </label>
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
        rowKey={(row) => row.preAdviceId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAttention={(row) => lateOf(row, now) !== ''}
        rowAlign="baseline"
        noMatchText={noMatchText}
        onClearFilters={narrowed ? clearFilters : undefined}
        renderDetail={(key) => (
          // 不按行重挂：重挂会把整栏卸掉再从加载骨架画起。换行留着上一张，新的读到再换。
          <PreAdviceDocument
            ownerAccount={owner}
            preAdviceId={key}
            panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }}
          />
        )}
        viewState={viewState}
        listFooter={owner !== '' && shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}
