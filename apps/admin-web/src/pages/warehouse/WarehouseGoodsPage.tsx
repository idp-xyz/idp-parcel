import { useEffect, useState } from 'react';
import { Plus, RefreshCw } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useSettledKeyword } from '../use-catalogue-cursor';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listOwnerGoods, type ApiResult, type GoodsListBody, type GoodsRecord } from './api';
import { DocumentLink } from './document-page';
import { GoodsDocument } from './GoodsDocument';
import { revisionPrefill } from './goods-profile';
import { openDocument, useDocumentParts } from './use-document';

const moduleId = 'warehouse-goods';
const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const columns: ListColumn<GoodsRecord>[] = [
  {
    id: 'code',
    header: '商品编码',
    className: 'whitespace-nowrap',
    render: (row) => <DocumentLink moduleId={moduleId} parts={[row.ownerAccount, row.goodsCode]} />,
  },
  { id: 'name', header: '名称', render: (row) => row.name },
  { id: 'unit', header: '基本单位', render: (row) => row.baseUnit },
  { id: 'track', header: '批次 / 效期', render: (row) => `${row.trackBatch ? '记批' : '不记批'} / ${row.trackExpiry ? '记效期' : '不记效期'}` },
  { id: 'composite', header: '组合商品', render: (row) => (row.composite ? '是' : '否') },
  { id: 'rev', header: '修订', className: mono, render: (row) => `r${row.revision}` },
  { id: 'at', header: '登记时刻', className: mono, render: (row) => formatInstant(row.registeredAt) },
];

/** 货主与商品：列表与每一个商品档案各占自己的标签。商品读口要货主，所以单据键是货主加商品编码两段。 */
export function WarehouseGoodsPage() {
  const parts = useDocumentParts();
  return parts ? <GoodsDocument ownerAccount={parts[0]} goodsCode={parts[1] ?? ''} /> : <GoodsList />;
}

function GoodsList() {
  const drawer = useActionDrawer(receivingActionSpecs);
  const owners = drawer.lookups.owners;
  const [picked, setPicked] = useState('');
  const [keyword, setKeyword] = useState('');
  const [answer, setAnswer] = useState<ApiResult<GoodsListBody> | null>(null);
  const [refreshes, setRefreshes] = useState(0);
  const refresh = () => setRefreshes((count) => count + 1);
  const settled = useSettledKeyword(keyword);

  // 商品按货主归属，读口必须带货主：人没挑过就取名册里的第一家，不让人对着一张空表找筛选。名册一到这一帧就有货主，
  // 不靠副作用补选——那样名册到了与选定之间会闪一帧「还没有货主客户账户」。
  const owner = picked !== '' ? picked : (owners[0]?.accountId ?? '');

  useEffect(() => {
    if (owner === '') {
      setAnswer(null);
      return;
    }
    let cancelled = false;
    setAnswer(null);
    void listOwnerGoods(owner, settled).then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [owner, settled, refreshes]);

  const rows = answer?.kind === 'outcome' ? answer.body.goods : [];
  const page = answer?.kind === 'outcome' ? answer.body.page : undefined;

  const inspectorOf = (row: GoodsRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '打开商品档案', onRun: () => openDocument(moduleId, [row.ownerAccount, row.goodsCode]) }];
    if (drawer.has('goods-on-behalf')) {
      actions.push({ label: `代录新修订（基于 r${row.revision}）`, onRun: () => drawer.open('goods-on-behalf', revisionPrefill(row)) });
    }
    return {
      title: row.name,
      subtitle: row.goodsCode,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主', value: row.ownerAccount, mono: true },
            { label: '基本单位', value: row.baseUnit },
            { label: '批号管理', value: row.trackBatch ? '强制跟踪' : '不跟踪' },
            { label: '效期管理', value: row.trackExpiry ? '强制跟踪' : '不跟踪' },
            { label: '组合商品', value: row.composite ? '是' : '否' },
            { label: '当前修订', value: `r${row.revision}`, mono: true },
          ],
        },
        { kind: 'actions', actions },
        {
          kind: 'audit',
          fields: [
            { label: '登记人', value: row.registrant, mono: true },
            { label: '登记时刻', value: formatInstant(row.registeredAt), mono: true },
          ],
        },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<GoodsRecord>
        moduleId={moduleId}
        title={info.title}
        description="按货主查阅商品当前修订。点商品编码打开商品档案：包装、别名、组合组件、尺寸重量，和它在各仓的库存。"
        headerActions={
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" onClick={refresh}>
              <RefreshCw className="mr-1 h-3.5 w-3.5" />
              刷新
            </Button>
            {drawer.has('goods-on-behalf') ? (
              <Button
                type="button"
                className={primaryActionClass}
                onClick={() => drawer.open('goods-on-behalf', { ownerAccount: owner, basisRevision: '0' })}
              >
                <Plus className="mr-1 h-3.5 w-3.5" />
                代录新商品
              </Button>
            ) : null}
          </div>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜商品编码或名称' }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
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
                    {account.customerPartyName ? `${account.accountId} · ${account.customerPartyName}` : account.accountId}
                  </option>
                ))}
              </select>
            </label>
          </div>
        }
        // 商品读口没有翻页条：一位货主的商品多过一页时，如实说只显示了前面这些，请用检索词找。
        filterSummary={
          page && page.next !== null ? (
            <span className="type-caption text-amber-600 dark:text-amber-400">只显示了前 {rows.length} 个，后面的请用检索词找</span>
          ) : undefined
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => `${row.ownerAccount}/${row.goodsCode}`}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [row.ownerAccount, row.goodsCode])}
        emptyRowsNote="该货主名下没有匹配的商品"
        viewState={
          owner === ''
            ? drawer.lookups.loaded
              ? {
                  kind: 'empty',
                  title: '还没有货主客户账户',
                  description: '商品按货主归属。先在国际小包那边的「客户」里登记货主的客户账户。',
                }
              : { kind: 'loading', shape: 'table' }
            : catalogueViewState(answer, rows.length, refresh, {
                module: info,
                endpoint: 'GET /warehouse-fulfillment-goods',
                emptyTitle: '该货主还没有商品档案',
                emptyDescription: '点右上角「代录新商品」为该货主建立首个商品修订。',
                narrowed: settled.trim() !== '',
              })
        }
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}
