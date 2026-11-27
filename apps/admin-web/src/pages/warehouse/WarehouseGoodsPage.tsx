import { useMemo, useState, type ReactNode } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import type { CatalogueConditions } from '../catalogue-query';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { listOwnerGoods, type GoodsListBody, type GoodsRecord } from './api';
import { DocumentLink } from './document-page';
import { GoodsDocument } from './GoodsDocument';
import { revisionPrefill } from './goods-profile';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { useDocumentParts } from './use-document';
import { WmsTwoLine } from './wms-ui';

const moduleId = 'warehouse-goods';
const info = moduleInfoById[moduleId];
const codeKey = 'goodsCode';
const noRows: GoodsRecord[] = [];

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

const emptyBook: GoodsListBody = { outcome: 'GOODS_LISTED', ownerAccount: '', goods: [], page: { size: 0, next: null, total: null } };

function loadGoods(conditions: CatalogueConditions, after: string | null) {
  const owner = conditions.selectors?.ownerAccount ?? '';
  if (owner === '') return Promise.resolve({ kind: 'outcome' as const, status: 200, body: emptyBook });
  return listOwnerGoods(owner, conditions.q ?? '', after, conditions.pageSize, conditions.sort);
}

/** 读口只按商品编码排，缺省从低到高。人点过表头就照他的。 */
function codeSort(stored: string | null): WorkbenchSort {
  if (stored === `-${codeKey}`) return { key: codeKey, dir: -1 };
  return { key: codeKey, dir: 1 };
}

function goodsKey(row: GoodsRecord): string {
  return `${row.ownerAccount}/${row.goodsCode}`;
}

/** 货主与商品：列表与每一个商品档案各占自己的标签。商品读口要货主，所以单据键是货主加商品编码两段。 */
export function WarehouseGoodsPage() {
  const parts = useDocumentParts();
  return parts ? <GoodsDocument ownerAccount={parts[0]} goodsCode={parts[1] ?? ''} /> : <GoodsList />;
}

/**
 * 按一位货主看他的商品。读口不收「全部货主」，名册还没到就先空着，到了还没人选就用名册里的第一位。
 * 点一行在右边开这个商品的档案，编码仍进整页。行上只留代录新修订。
 */
function GoodsList() {
  const drawer = useActionDrawer(receivingActionSpecs);
  const owners = drawer.lookups.owners;
  const [picked, setPicked] = useTabViewState('warehouse-goods.owner', '');
  const [keyword, setKeyword] = useTabViewState('warehouse-goods.keyword', '');
  const [storedSort, setSort] = useTabViewState<string | null>('warehouse-goods.sort', null);
  const [openKey, setOpenKey] = useTabViewState<string | null>('warehouse-goods.open', null);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(keyword);
  const owner = picked !== '' ? picked : (owners[0]?.accountId ?? '');
  const sort = codeSort(storedSort);
  const { answer, previous, pagination, retry } = useCatalogueCursor(
    { selectors: { ownerAccount: owner }, q: settled, sort: sort.dir === 1 ? codeKey : `-${codeKey}` },
    loadGoods,
  );
  const held = heldCataloguePage(answer, previous, (body) => body.goods.length > 0 && body.goods.every((row) => row.ownerAccount === owner));
  const rows = answer?.kind === 'outcome' ? answer.body.goods : (held?.body.goods ?? noRows);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const ownerNames = useMemo(
    () => new Map(owners.map((account) => [account.accountId, account.customerPartyName ?? ''])),
    [owners],
  );
  const now = new Date();
  const ownerName = nameOf(ownerNames, owner);

  const columns: WorkbenchColumn<GoodsRecord>[] = [
    {
      key: 'goods',
      header: '商品',
      sortKey: codeKey,
      render: (row) => (
        <WmsTwoLine
          fill
          primary={row.name}
          secondary={<DocumentLink moduleId={moduleId} parts={[row.ownerAccount, row.goodsCode]} />}
          title={`${row.name}\n${row.goodsCode}`}
        />
      ),
    },
    { key: 'unit', header: '单位', width: fitColumnWidth, render: (row) => nowrap(row.baseUnit) },
    {
      key: 'track',
      header: nowrap('批次 / 效期'),
      width: fitColumnWidth,
      minListWidth: 860,
      render: (row) => nowrap(`${row.trackBatch ? '记批' : '不记批'} / ${row.trackExpiry ? '记效期' : '不记效期'} / ${row.trackSerial ? '记序列号' : '不记序列号'}`),
    },
    { key: 'composite', header: '组合', width: fitColumnWidth, minListWidth: 1040, render: (row) => nowrap(row.composite ? '是' : '否') },
    { key: 'rev', header: '修订', width: fitColumnWidth, render: (row) => <span className="whitespace-nowrap tabular-nums">{`r${row.revision}`}</span> },
    {
      key: 'at',
      header: nowrap('登记'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1180,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.registeredAt)}>
          {shortInstant(row.registeredAt, now) || '—'}
        </span>
      ),
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <GoodsStep row={row} drawer={drawer} /> },
  ];

  const narrowed = settled.trim() !== '' || pastFirstPage(pagination);
  const clearFilters = () => {
    setKeyword('');
    setSort(null);
  };
  const viewState: TemplateViewState =
    owner === ''
      ? drawer.lookups.loaded
        ? { kind: 'empty', title: '还没有货主客户账户', description: '商品按货主归属。先在国际小包那边的「客户」里登记货主的客户账户。' }
        : { kind: 'loading', shape: 'table' }
      : held
        ? { kind: 'ready' }
        : catalogueViewState(answer, rows.length, refresh, {
            module: info,
            endpoint: 'GET /warehouse-fulfillment-goods',
            emptyTitle: '该货主还没有商品档案',
            emptyDescription: '点右上角「代录新商品」为该货主建立首个商品修订。',
            narrowed,
          });
  const shownPagination = owner === '' ? undefined : (pagination ?? held?.pagination);

  return (
    <>
      <WorkbenchPageTemplate<GoodsRecord>
        title={info.title}
        viewStateKey={moduleId}
        subtitle={owner === '' ? '先有货主，才能看他的商品' : `正在看${ownerName || owner}的商品。点一行在右边看档案`}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          drawer.has('goods-on-behalf') && owner !== '' ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('goods-on-behalf', { ownerAccount: owner, basisRevision: '0' })}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              代录新商品
            </Button>
          ) : undefined
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜商品编码或名称' }}
        extraFilters={
          <>
            <label className="inline-flex items-center gap-1.5 type-body text-idpxyz-textMuted">
              <span className="whitespace-nowrap">货主</span>
              <select
                className="h-7 max-w-[16rem] rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text focus:border-idpxyz-accent focus:outline-none"
                value={owner}
                onChange={(event) => {
                  setPicked(event.target.value);
                  setOpenKey(null);
                }}
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
        rowKey={goodsKey}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => {
          const next = toggleWorkbenchSort(sort, key);
          setSort(next.dir === 1 ? codeKey : `-${codeKey}`);
        }}
        noMatchText="该货主名下没有匹配的商品"
        onClearFilters={clearFilters}
        renderDetail={(key) => {
          const splitAt = key.indexOf('/');
          return (
            <GoodsDocument
              ownerAccount={key.slice(0, splitAt)}
              goodsCode={key.slice(splitAt + 1)}
              panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }}
            />
          );
        }}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

function GoodsStep({ row, drawer }: { row: GoodsRecord; drawer: ActionDrawerControl }) {
  if (!drawer.has('goods-on-behalf')) return <span className="text-idpxyz-textMuted">—</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        drawer.open('goods-on-behalf', revisionPrefill(row));
      }}
    >
      代录新修订
    </Button>
  );
}
