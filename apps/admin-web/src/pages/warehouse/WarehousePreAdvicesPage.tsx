import { useEffect, useState } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { enterStation } from '../../shell/enter-station';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useSettledKeyword } from '../use-catalogue-cursor';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listOwnerPreAdvices, type ApiResult, type PreAdviceListBody, type PreAdviceRecord } from './api';
import { DocumentLink } from './document-page';
import { PreAdviceDocument } from './PreAdviceDocument';
import { openDocument, useDocumentParts } from './use-document';
import { receiveStationHash } from './station/stations';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsStatusTabs } from './wms-ui';

const moduleId = 'warehouse-pre-advices';
const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const statuses = ['ADVISED', 'RECEIVING', 'CLOSED', 'CANCELLED'] as const;

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
  const [owner, setOwner] = useState('');
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState<string>('ALL');
  const [answer, setAnswer] = useState<ApiResult<PreAdviceListBody> | null>(null);
  const [refreshes, setRefreshes] = useState(0);
  const refresh = () => setRefreshes((count) => count + 1);
  const settled = useSettledKeyword(keyword);

  // 预报按货主归属，读口必须带货主：名册到了先选第一家，不让人对着一张空表找筛选。
  useEffect(() => {
    if (owner === '' && owners.length > 0) setOwner(owners[0].accountId);
  }, [owners, owner]);

  useEffect(() => {
    if (owner === '') {
      setAnswer(null);
      return;
    }
    let cancelled = false;
    setAnswer(null);
    void listOwnerPreAdvices(owner, settled).then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [owner, settled, refreshes]);

  const loaded = answer?.kind === 'outcome' ? answer.body.preAdvices : [];
  // 读口不按状态筛，状态是在答回来的这一页里筛的。这一页就是全部时签上才写数；后面还有页的话，数出来的不是各队的总数。
  const wholeList = answer?.kind === 'outcome' && answer.body.page.next === null;
  const rows = status === 'ALL' ? loaded : loaded.filter((row) => row.status === status);
  const now = new Date();

  const columns: ListColumn<PreAdviceRecord>[] = [
    {
      id: 'id',
      header: '入库单号',
      className: 'whitespace-nowrap',
      render: (row) => (
        <DocumentLink moduleId={moduleId} parts={[row.ownerAccount, row.preAdviceId]}>
          {row.documentNumber ?? row.preAdviceId}
        </DocumentLink>
      ),
    },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} /> },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'mode', header: '到仓 / 入库', className: 'whitespace-nowrap', render: (row) => `${labelOf(row.arrivalMode)} · ${labelOf(row.inboundMode)}` },
    {
      id: 'size',
      header: '商品行 / 整箱',
      align: 'right',
      className: mono,
      render: (row) => `${row.goodsLines?.length ?? 0} / ${row.cartons?.length ?? 0}`,
    },
    {
      id: 'expected',
      header: '预计到达',
      className: 'whitespace-nowrap',
      render: (row) => {
        const late = lateOf(row, now);
        return (
          <span className="inline-flex items-center gap-2">
            <span className="font-mono">{row.currentNotice ? formatInstant(row.currentNotice.expectedAt) : '—'}</span>
            {late ? <WmsBadge label={`已迟 ${late}`} tone="danger" /> : null}
          </span>
        );
      },
    },
    { id: 'at', header: '受理时刻', className: mono, render: (row) => formatInstant(row.acceptedAt) },
    {
      id: 'next',
      header: '下一步',
      className: 'whitespace-nowrap',
      render: (row) =>
        row.status === 'ADVISED' || row.status === 'RECEIVING' ? (
          <button
            type="button"
            className="text-[color:var(--ds-text-link-hover)] hover:underline"
            onClick={(event) => {
              event.stopPropagation();
              enterStation(receiveStationHash(row.preAdviceId));
            }}
          >
            {row.status === 'ADVISED' ? '去收货' : '接着收货'}
          </button>
        ) : (
          <span className="text-idpxyz-textMuted">—</span>
        ),
    },
  ];

  const inspectorOf = (row: PreAdviceRecord): InspectorContent => {
    const actions: InspectorAction[] = [{ label: '打开入库预报', onRun: () => openDocument(moduleId, [row.ownerAccount, row.preAdviceId]) }];
    const open = row.status === 'ADVISED' || row.status === 'RECEIVING';
    if (open && drawer.has('pre-advice-notice')) {
      actions.push({ label: '更新告知', onRun: () => drawer.open('pre-advice-notice', { preAdviceId: row.preAdviceId }) });
    }
    if (row.status === 'ADVISED' && drawer.has('pre-advice-withdrawal')) {
      actions.push({ label: '撤回', onRun: () => drawer.open('pre-advice-withdrawal', { preAdviceId: row.preAdviceId }) });
    }
    if (row.status === 'RECEIVING' && drawer.has('pre-advice-close')) {
      actions.push({ label: '完结', onRun: () => drawer.open('pre-advice-close', { preAdviceId: row.preAdviceId }) });
    }
    return {
      title: row.documentNumber ?? row.preAdviceId,
      subtitle: labelOf(row.status),
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主 / 仓库', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
            { label: '到仓 / 入库', value: `${labelOf(row.arrivalMode)} · ${labelOf(row.inboundMode)}` },
            { label: '预计到达', value: row.currentNotice ? formatInstant(row.currentNotice.expectedAt) : '没有告知', mono: true },
            {
              label: '商品',
              value: row.goodsLines?.map((line) => `${line.goodsCode}×${line.quantity}`).join('、') || '没有按商品的行',
              mono: true,
            },
            { label: '整箱', value: row.cartons?.map((carton) => carton.cartonNumber).join('、') || '没有整箱', mono: true },
          ],
        },
        { kind: 'actions', actions },
        {
          kind: 'audit',
          fields: [{ label: '来源 / 受理时刻', value: `${labelOf(row.submissionPath)} · ${formatInstant(row.acceptedAt)}` }],
        },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<PreAdviceRecord>
        moduleId={moduleId}
        title={info.title}
        description="货主说要来的货。点单号看预报了多少、收了多少和每一次收货；收货本身在作业台做。"
        headerActions={
          drawer.has('pre-advice-on-behalf') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('pre-advice-on-behalf', { ownerAccount: owner })}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              代录入库预报
            </Button>
          ) : undefined
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜预报标识或仓编码' }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={status}
              onChange={setStatus}
              tabs={[
                { value: 'ALL', label: '全部', count: wholeList ? loaded.length : undefined },
                ...statuses.map((code) => ({
                  value: code,
                  label: labelOf(code),
                  count: wholeList ? loaded.filter((row) => row.status === code).length : undefined,
                })),
              ]}
            />
            <label className="inline-flex items-center gap-1.5 type-body text-idpxyz-textMuted">
              <span className="whitespace-nowrap">货主</span>
              <select
                className="h-7 max-w-[16rem] rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text focus:border-idpxyz-accent focus:outline-none"
                value={owner}
                onChange={(event) => setOwner(event.target.value)}
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
        columns={columns}
        rows={rows}
        rowKey={(row) => row.preAdviceId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [row.ownerAccount, row.preAdviceId])}
        emptyRowsNote={
          wholeList ? '这位货主名下没有这个状态的入库预报' : '这一页里没有这个状态的入库预报；读口不按状态筛，后面的页没有看'
        }
        viewState={
          owner === ''
            ? drawer.lookups.loaded
              ? {
                  kind: 'empty',
                  title: '还没有货主客户账户',
                  description: '入库预报按货主归属。先在国际小包那边的「客户」里登记货主的客户账户。',
                }
              : { kind: 'loading', shape: 'table' }
            : catalogueViewState(answer, loaded.length, refresh, {
                module: info,
                endpoint: 'GET /warehouse-fulfillment-inbound-pre-advices',
                emptyTitle: '这位货主还没有入库预报',
                emptyDescription: '点右上角「代录入库预报」新建，或等货主经客户接口提交。',
                narrowed: settled.trim() !== '',
              })
        }
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}
