import { useEffect, useRef, type ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { formatInstant } from '../catalogue-view';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { getGoodsCurrent, queryOwnerStock, type GoodsRecord, type StockReadBody } from './api';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { goodsMeasurementRows, goodsStockRows, revisionPrefill } from './goods-profile';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { openDocument, useReturnToList } from './use-document';
import { labelOf, WmsBadge, WmsInfoGrid, WmsSubTable, type WmsSubColumn } from './wms-ui';

const moduleId = 'warehouse-goods';

type StockSummaryRow = NonNullable<StockReadBody['summary']>[number];
type AliasRow = NonNullable<GoodsRecord['aliases']>[number];

// 商品读口要货主加商品编码两格；useKeyedLoad 只认一把字符串键，两格用换行拼起来（与入库预报页同一手法）。
function loadGoods(key: string) {
  const splitAt = key.indexOf('\n');
  return getGoodsCurrent(key.slice(0, splitAt), key.slice(splitAt + 1));
}

// 库存汇总按货主问、答这位货主名下全部商品的汇总，所以键只是货主；这个商品的行在页面上按编码筛出。
function loadOwnerStock(ownerAccount: string) {
  return queryOwnerStock(ownerAccount, '', 'summary');
}

/** 库存块还没有行可摆时的一句话：在读、读不到都不画空表头。 */
function BlockNote({ title, tone, children }: { title: string; tone?: 'danger'; children: ReactNode }) {
  return (
    <section className="space-y-2">
      <h3 className="type-body font-semibold text-idpxyz-textBright">{title}</h3>
      <p
        className={`rounded-lg border px-4 py-5 type-body ${
          tone === 'danger'
            ? 'border-[color:var(--ds-status-danger-border)] bg-[color:var(--ds-status-danger-bg-subtle)] text-idpxyz-text'
            : 'border-idpxyz-border bg-idpxyz-sidebar text-idpxyz-textMuted'
        }`}
      >
        {children}
      </p>
    </section>
  );
}

const validityWord = (validity: { from?: string; to?: string } | undefined): string => {
  if (!validity) return '—';
  const parts = [validity.from ? `${formatInstant(validity.from)} 起` : '', validity.to ? `${formatInstant(validity.to)} 止` : ''];
  return parts.filter((part) => part !== '').join(' ') || '—';
};

/**
 * 商品档案页：这位货主这个商品当前修订的全部资料，加上它此刻在各仓的库存。商品没有生命周期，只有修订号一路往上加，
 * 所以头区用修订徽章，不画进度条；改资料是代录一个新修订。
 *
 * 同一张页也摆在列表的详情栏里（给了 `panel`）：换一个商品留着上一份，新的读到再换。列表重取会让 `revision` 变，档案跟着重读。
 */
export function GoodsDocument({
  ownerAccount,
  goodsCode,
  panel,
}: {
  ownerAccount: string;
  goodsCode: string;
  panel?: { onClose: () => void; onChanged: () => void; revision: number };
}) {
  const drawer = useActionDrawer(receivingActionSpecs);
  const back = useReturnToList(moduleId);
  const complete = ownerAccount !== '' && goodsCode !== '';
  const goodsKey = complete ? `${ownerAccount}\n${goodsCode}` : null;
  const goodsLoad = useKeyedLoad(goodsKey, loadGoods, { holdPrevious: panel !== undefined });
  const stockLoad = useKeyedLoad(complete ? ownerAccount : null, loadOwnerStock, { holdPrevious: panel !== undefined });
  const reloadGoods = goodsLoad.reload;
  const reloadStock = stockLoad.reload;
  const reload = () => {
    reloadGoods();
    reloadStock();
  };
  const shownKey = useRef(goodsKey);
  useEffect(() => {
    if (shownKey.current === goodsKey) return;
    shownKey.current = goodsKey;
    drawer.close();
  }, [goodsKey, drawer]);
  const revision = panel?.revision;
  const readAt = useRef(revision);
  useEffect(() => {
    if (readAt.current === revision) return;
    readAt.current = revision;
    reloadGoods();
    reloadStock();
  }, [revision, reloadGoods, reloadStock]);

  const { answer, loading, held, reloading } = goodsLoad;
  const goods: GoodsRecord | undefined = answer?.kind === 'outcome' ? answer.body.goods : undefined;
  const failure =
    (complete ? null : '地址里缺货主或商品编码：商品的地址是「货主 / 商品编码」两段。') ??
    readFailureMessage(answer) ??
    (answer?.kind === 'outcome' && !goods ? `货主 ${ownerAccount} 名下没有 ${goodsCode} 这个商品。` : null);

  const stockFailure = readFailureMessage(stockLoad.answer);
  const stockRows = goodsStockRows(stockLoad.answer?.kind === 'outcome' ? stockLoad.answer.body.summary : undefined, goods?.goodsCode ?? goodsCode);
  const stockColumns: WmsSubColumn<StockSummaryRow>[] = [
    { id: 'warehouse', header: '仓', mono: true, render: (row) => row.warehouse },
    { id: 'unit', header: '单位', render: (row) => row.unit },
    { id: 'state', header: '库存状态', render: (row) => <WmsBadge code={row.stockState} /> },
    { id: 'onHand', header: '在库', align: 'right', mono: true, render: (row) => String(row.onHand) },
    { id: 'allocatable', header: '可分配', align: 'right', mono: true, render: (row) => String(row.allocatable) },
    { id: 'frozen', header: '冻结', align: 'right', mono: true, render: (row) => String(row.frozen) },
    { id: 'reserved', header: '预占', align: 'right', mono: true, render: (row) => String(row.reserved) },
  ];
  // 作业锁是在库里扣掉的第四样；没有一行带它时不摆这一列，免得一列全是 0。有的话不摆就对不上「可用」。
  if (stockRows.some((row) => row.locked > 0)) {
    stockColumns.push({ id: 'locked', header: '作业锁', align: 'right', mono: true, render: (row) => String(row.locked) });
  }
  stockColumns.push({ id: 'available', header: '可用', align: 'right', mono: true, render: (row) => String(row.available) });

  const levels = goods?.packagingLevels ?? [];
  const aliases = goods?.aliases ?? [];
  const aliasColumns: WmsSubColumn<AliasRow>[] = [
    { id: 'code', header: '编码', mono: true, render: (alias) => alias.code },
    { id: 'type', header: '种类', render: (alias) => labelOf(alias.type) },
    { id: 'platform', header: '平台', render: (alias) => alias.platform || '—' },
  ];
  // 有效期是少数别名才有的格，都没有时不摆这一列。
  if (aliases.some((alias) => alias.validity?.from || alias.validity?.to)) {
    aliasColumns.push({ id: 'validity', header: '有效期', render: (alias) => validityWord(alias.validity) });
  }
  const components = goods?.composition?.components ?? [];
  const declaration = goods?.declaration;
  const measurements = goods ? goodsMeasurementRows(goods) : [];

  return (
    <>
      <WmsDocumentPage
        kind="商品档案"
        measure="份"
        title={goods?.goodsCode ?? goodsCode}
        subtitle={
          goods
            ? [`货主 ${goods.ownerAccount}`, goods.name, `基本单位 ${goods.baseUnit}`, goods.composite ? '组合商品' : ''].filter((part) => part !== '').join(' · ')
            : undefined
        }
        badge={goods ? <WmsBadge label={`修订 r${goods.revision}`} tone="info" /> : undefined}
        loading={held ? false : loading}
        reloading={reloading}
        busy={held}
        error={failure}
        onBack={back}
        onReload={reload}
        panel={panel ? { onClose: panel.onClose, onOpenPage: () => openDocument(moduleId, [ownerAccount, goods?.goodsCode ?? goodsCode]) } : undefined}
        actions={
          goods && drawer.has('goods-on-behalf') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('goods-on-behalf', revisionPrefill(goods))}>
              代录新修订
            </Button>
          ) : undefined
        }
        aside={
          goods && (goods.submissionPath || goods.evidenceReference) ? (
            <WmsAsideBlock title="来源">
              {goods.submissionPath ? <span>{labelOf(goods.submissionPath)}</span> : null}
              {goods.evidenceReference ? <span className="break-all type-caption text-idpxyz-textMuted">依据 {goods.evidenceReference}</span> : null}
              <span className="type-caption text-idpxyz-textMuted">
                修订 r{goods.revision} 登记于 {formatInstant(goods.registeredAt)}
              </span>
            </WmsAsideBlock>
          ) : undefined
        }
      >
        {goods ? (
          <>
            <WmsInfoGrid
              items={[
                { label: '货主', value: goods.ownerAccount, mono: true },
                { label: '名称', value: goods.name },
                { label: '基本单位', value: goods.baseUnit },
                { label: '批次', value: goods.trackBatch ? '记批' : '不记批' },
                { label: '效期', value: goods.trackExpiry ? '记效期' : '不记效期' },
                { label: '组合', value: goods.composite ? '是' : '否' },
                { label: '登记', value: `${goods.registrant} · ${formatInstant(goods.registeredAt)}` },
              ]}
            />
            {stockLoad.loading ? (
              <BlockNote title="库存">正在取这位货主的库存…</BlockNote>
            ) : stockFailure ? (
              <BlockNote title="库存" tone="danger">
                没有取到库存：{stockFailure}
              </BlockNote>
            ) : (
              <WmsSubTable
                title="库存"
                count={stockRows.length}
                emptyText="这位货主名下，这个商品在各仓都没有库存。"
                rows={stockRows}
                rowKey={(row) => `${row.warehouse}/${row.unit}/${row.stockState}`}
                columns={stockColumns}
              />
            )}
            <WmsSubTable
              title="包装层级"
              count={levels.length}
              emptyText="没有登记包装层级。"
              rows={levels}
              rowKey={(level) => level.name}
              columns={[
                { id: 'name', header: '层级', render: (level) => level.name },
                { id: 'factor', header: '含基本单位', align: 'right', mono: true, render: (level) => String(level.factor) },
              ]}
            />
            <WmsSubTable
              title="别名"
              count={aliases.length}
              emptyText="没有登记条码或平台码。"
              rows={aliases}
              rowKey={(alias, index) => `${alias.code}-${index}`}
              columns={aliasColumns}
            />
            {goods.composite ? (
              <WmsSubTable
                title="组合组件"
                count={components.length}
                extra={<span className="type-caption text-idpxyz-textMuted">{labelOf(goods.composition?.outboundMode)}</span>}
                emptyText="没有登记组件。"
                rows={components}
                rowKey={(component) => component.code}
                columns={[
                  {
                    id: 'code',
                    header: '组件',
                    // 组件是同一位货主名下的另一个商品。
                    render: (component) => <DocumentLink moduleId={moduleId} parts={[goods.ownerAccount, component.code]} />,
                  },
                  { id: 'qty', header: '每件用量', align: 'right', mono: true, render: (component) => String(component.quantity) },
                ]}
              />
            ) : null}
            {declaration ? (
              <section className="space-y-2">
                <h3 className="type-body font-semibold text-idpxyz-textBright">申报资料</h3>
                <WmsInfoGrid
                  items={[
                    { label: '申报品名', value: declaration.name },
                    { label: '税则号', value: declaration.hsCode, mono: true },
                    { label: '原产国', value: declaration.originCountry },
                    { label: '申报单价', value: declaration.unitPrice ? `${declaration.unitPrice} ${declaration.currency ?? ''}`.trim() : undefined, mono: true },
                  ]}
                />
              </section>
            ) : null}
            <WmsSubTable
              title="申报与仓测尺寸重量"
              count={measurements.length}
              emptyText="没有登记申报的尺寸重量，也没有仓测记录。"
              rows={measurements}
              rowKey={(row) => row.level}
              columns={[
                { id: 'level', header: '层级', render: (row) => row.level },
                { id: 'declared', header: '货主申报', mono: true, render: (row) => row.declared || '—' },
                {
                  id: 'measured',
                  header: '仓测',
                  render: (row) => (row.measured ? <span className="whitespace-nowrap font-mono">{row.current || '—'}</span> : <span className="text-idpxyz-textMuted">未仓测</span>),
                },
                {
                  id: 'by',
                  header: '仓测登记',
                  render: (row) =>
                    row.measuredBy ? `${row.measuredBy.warehouseCode} · ${row.measuredBy.operator} · ${formatInstant(row.measuredBy.occurredAt)}` : '—',
                },
              ]}
            />
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={panel ? panel.onChanged : reload} />
    </>
  );
}
