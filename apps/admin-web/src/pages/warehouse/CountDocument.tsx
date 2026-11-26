import type { ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { enterStation } from '../../shell/enter-station';
import { formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { getCountDetail, type StockCountDetailBody } from './api';
import { countSheet } from './print-forms';
import { openPrint } from './print-sheet';
import { countStationHash } from './station/stations';
import {
  codesPreview,
  countEntryRows,
  countProgress,
  countStatusLabel,
  countTrack,
  differenceDirection,
  differenceDirectionLabels,
  differenceGap,
  differenceSummary,
  differenceTotals,
  effectiveEntryIds,
  lotLabel,
  mismatchedDifferences,
  signedGap,
  type CountEntryRow,
} from './count-flow';
import { countsAdjustmentsModule } from './counts-adjustments-address';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { discrepancyStatusLabel } from './stock-approval-flow';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { useReturnToList } from './use-document';
import { WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable, type WmsSubColumn, type WmsTone } from './wms-ui';

const moduleId = countsAdjustmentsModule;

type DifferenceRecord = NonNullable<StockCountDetailBody['differences']>[number];

const mutedText = 'type-caption text-idpxyz-textMuted';

/** 批准或驳回那一刻留下的：谁、几时、写了什么。 */
function decidedNote(row: DifferenceRecord): string {
  const parts =
    row.status === 'APPROVED'
      ? [row.approvedBy, row.approvedAt ? formatInstant(row.approvedAt) : '', row.approvalNote]
      : row.status === 'REJECTED'
        ? [row.rejectedBy, row.rejectedAt ? formatInstant(row.rejectedAt) : '', row.rejectReason]
        : [];
  return parts.filter((part) => part !== undefined && part !== '').join(' · ');
}

/**
 * 盘点页：一份盘点从开始到差异批完的全部。盘点的价值在差异——账面与清点不一致的行单独列出，标明盘盈还是盘亏、差多少，
 * 一致的行不混进去；清点录入是这些差异的依据，每个库位以最后一次为准。开始、结束并出差异与作废在头区按状态做，
 * 批准与驳回落在每一条差异上（一次只能批一条，所以按钮跟着行走）。
 */
export function CountDocument({ countId }: { countId: string }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const back = useReturnToList(moduleId);
  const complete = countId !== '';
  const { answer, loading, reload } = useKeyedLoad(complete ? countId : null, getCountDetail);
  const body: StockCountDetailBody | undefined = answer?.kind === 'outcome' ? answer.body : undefined;
  const count = body?.count;
  const failure =
    (complete ? null : '地址里缺盘点号：盘点的地址是「盘点 / 盘点号」两段。') ??
    readFailureMessage(answer) ??
    (body && !count ? `没有 ${countId} 这张盘点单。它可能属于别的公司，或盘点号有误。` : null);

  const entries = body?.entries ?? [];
  const differences = mismatchedDifferences(body?.differences ?? []);
  const totals = differenceTotals(differences);
  const entryRows = countEntryRows(entries);
  const supersededEntries = entries.length - effectiveEntryIds(entries).size;
  const countedLocations = new Set(entries.map((entry) => entry.locationCode));
  const progress = count ? countProgress(count.locations, [...countedLocations]) : null;
  const track = count ? countTrack(count.status, totals.pendingRows) : null;
  const status = count?.status;
  const allCounted = progress !== null && progress.missing.length === 0;
  const mayDecide = drawer.has('count-approve') || drawer.has('count-reject');

  const goCount = () => {
    if (count) enterStation(countStationHash(count.countId));
  };
  // 主动作排在最后：还有库位没录就是去作业台，都录齐了就是结束。
  const workstationButton =
    status === 'COUNTING' ? (
      <Button type="button" variant={allCounted ? 'outline' : undefined} className={allCounted ? undefined : primaryActionClass} onClick={goCount}>
        去清点
      </Button>
    ) : null;
  const completeButton =
    count && status === 'COUNTING' && drawer.has('count-complete') ? (
      <Button
        type="button"
        variant={allCounted ? undefined : 'outline'}
        className={allCounted ? primaryActionClass : undefined}
        onClick={() => drawer.open('count-complete', { countId: count.countId })}
      >
        结束并出差异
      </Button>
    ) : null;

  const kpis: Array<{ label: string; value: string | number; note?: string; tone?: WmsTone }> = [];
  if (count && progress) {
    kpis.push({
      label: '库位清点',
      value: `${progress.counted} / ${progress.total}`,
      note:
        status === 'PENDING'
          ? '开始盘点后到作业台按库位录入'
          : progress.missing.length > 0
            ? status === 'VOID'
              ? `作废时还有 ${progress.missing.length} 个库位没录入`
              : `还差 ${progress.missing.length} 个库位`
            : '每个库位都有清点录入',
      tone: status === 'COUNTING' ? (progress.missing.length > 0 ? 'warning' : 'success') : undefined,
    });
    kpis.push({
      label: '清点笔数',
      value: count.entryCount,
      note: supersededEntries > 0 ? `其中 ${supersededEntries} 笔已被重录取代` : undefined,
    });
  }
  if (status === 'COUNTED') {
    kpis.push({
      label: '差异',
      value: totals.rows,
      note: differenceSummary(totals) || '账实一致，没有差异',
      tone: totals.rows > 0 ? 'warning' : 'success',
    });
    kpis.push({
      label: '待批差异',
      value: totals.pendingRows,
      note: totals.rows > 0 ? `${totals.rows - totals.pendingRows} 条已处理` : undefined,
      tone: totals.pendingRows > 0 ? 'warning' : undefined,
    });
  }

  const showOwner = count !== undefined && !count.ownerAccount;
  const differenceColumns: WmsSubColumn<DifferenceRecord>[] = [{ id: 'loc', header: '库位', mono: true, render: (row) => row.locationCode }];
  if (showOwner) differenceColumns.push({ id: 'owner', header: '货主', mono: true, render: (row) => row.ownerAccount });
  differenceColumns.push(
    { id: 'goods', header: '商品', render: (row) => <DocumentLink moduleId="warehouse-goods" parts={[row.ownerAccount, row.goodsCode]} /> },
    { id: 'state', header: '库存状态', render: (row) => <WmsBadge code={row.stockState} /> },
  );
  // 批号、效期与贴标只有少数行才有：没有一行带它时不摆这一列。
  if (differences.some((row) => lotLabel(row) !== '')) {
    differenceColumns.push({ id: 'lot', header: '批次', render: (row) => lotLabel(row) || '—' });
  }
  differenceColumns.push(
    { id: 'book', header: '账面', align: 'right', mono: true, render: (row) => String(row.bookQuantity) },
    { id: 'counted', header: '清点', align: 'right', mono: true, render: (row) => String(row.countedQuantity) },
    {
      id: 'gap',
      header: '盘盈 / 盘亏',
      mono: true,
      render: (row) => {
        const direction = differenceDirection(row);
        if (direction === null) return '—';
        return (
          <span className={direction === 'gain' ? 'text-[color:var(--ds-status-info-fg)]' : 'text-[color:var(--ds-status-danger-fg)]'}>
            {differenceDirectionLabels[direction]} {signedGap(differenceGap(row))} {row.unit}
          </span>
        );
      },
    },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} label={discrepancyStatusLabel(row.status)} /> },
    {
      id: 'act',
      header: '处理',
      render: (row) =>
        row.status === 'PENDING' ? (
          mayDecide ? (
            <span className="inline-flex gap-2">
              {drawer.has('count-approve') ? (
                <Button type="button" size="sm" variant="outline" onClick={() => drawer.open('count-approve', { differenceId: row.differenceId })}>
                  批准入账
                </Button>
              ) : null}
              {drawer.has('count-reject') ? (
                <Button type="button" size="sm" variant="outline" onClick={() => drawer.open('count-reject', { differenceId: row.differenceId })}>
                  驳回
                </Button>
              ) : null}
            </span>
          ) : (
            <span className={mutedText}>等批准</span>
          )
        ) : (
          <span className={mutedText}>{decidedNote(row) || '—'}</span>
        ),
    },
  );

  // 被后来的重录取代的那几行划掉：它们还在是为了留痕，不再进账实比对。
  const struck = (row: CountEntryRow, node: ReactNode): ReactNode =>
    row.superseded ? <span className="text-idpxyz-textMuted line-through">{node}</span> : node;
  const hasEntryNotes = entryRows.some((row) => row.superseded || row.observed !== undefined);
  const entryColumns: WmsSubColumn<CountEntryRow>[] = [
    { id: 'loc', header: '库位', mono: true, render: (row) => struck(row, row.locationCode) },
    {
      id: 'goods',
      header: '商品',
      render: (row) =>
        struck(
          row,
          row.empty ? (
            <span className={mutedText}>这个库位清点为空，没有货</span>
          ) : row.goodsCode ? (
            row.ownerAccount ? (
              <DocumentLink moduleId="warehouse-goods" parts={[row.ownerAccount, row.goodsCode]} />
            ) : (
              row.goodsCode
            )
          ) : row.observed ? (
            `未认出：${row.observed}`
          ) : (
            '—'
          ),
        ),
    },
    { id: 'state', header: '库存状态', render: (row) => struck(row, row.stockState ? <WmsBadge code={row.stockState} /> : '—') },
  ];
  if (entryRows.some((row) => lotLabel(row) !== '')) {
    entryColumns.push({ id: 'lot', header: '批次', render: (row) => struck(row, lotLabel(row) || '—') });
  }
  entryColumns.push(
    { id: 'qty', header: '清点数', align: 'right', mono: true, render: (row) => struck(row, row.unit ? `${row.quantity} ${row.unit}` : String(row.quantity)) },
    { id: 'who', header: '录入人', mono: true, render: (row) => struck(row, row.operatorId) },
    { id: 'at', header: '时刻', mono: true, render: (row) => struck(row, formatInstant(row.occurredAt)) },
  );
  if (hasEntryNotes) {
    entryColumns.push({
      id: 'note',
      header: '说明',
      render: (row) =>
        row.superseded ? (
          <WmsBadge label="已被重录取代" tone="neutral" />
        ) : row.observed !== undefined ? (
          <span className={mutedText}>现场认不出，不进账实比对</span>
        ) : (
          '—'
        ),
    });
  }

  return (
    <>
      <WmsDocumentPage
        kind="盘点单"
        title={count?.countId ?? countId}
        subtitle={
          count
            ? [
                `仓 ${count.warehouseCode}`,
                count.ownerAccount ? `货主 ${count.ownerAccount}` : '不限货主',
                `${count.operator} 创建于 ${formatInstant(count.createdAt)}`,
              ].join(' · ')
            : undefined
        }
        badge={
          count ? (
            <>
              <WmsBadge code={count.status} label={countStatusLabel(count.status)} />
              {status === 'COUNTED' && totals.pendingRows > 0 ? <WmsBadge label={`${totals.pendingRows} 条差异待批`} tone="warning" /> : null}
            </>
          ) : undefined
        }
        loading={loading}
        error={failure}
        onBack={back}
        onReload={reload}
        stepper={
          track ? (
            <WmsStepper
              steps={track.steps}
              currentId={track.currentId}
              currentLabel={track.currentLabel}
              failedLabel={track.halt?.label}
              failedTone={track.halt?.tone}
            />
          ) : undefined
        }
        actions={
          count ? (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  const keep = effectiveEntryIds(entries);
                  const lines = entries
                    .filter((entry) => keep.has(entry.factId))
                    .flatMap((entry) => entry.lines.map((line) => ({ locationCode: entry.locationCode, goodsCode: line.goodsCode ?? '', quantity: line.quantity, stockState: line.stockState, observed: line.observed })));
                  openPrint(countSheet(count.countId, count.warehouseCode, count.locations, lines));
                }}
              >
                打印盘点单
              </Button>
              {(status === 'PENDING' || status === 'COUNTING') && drawer.has('count-void') ? (
                <Button type="button" variant="outline" onClick={() => drawer.open('count-void', { countId: count.countId })}>
                  作废
                </Button>
              ) : null}
              {allCounted ? (
                <>
                  {workstationButton}
                  {completeButton}
                </>
              ) : (
                <>
                  {completeButton}
                  {workstationButton}
                </>
              )}
              {status === 'PENDING' && drawer.has('count-start') ? (
                <Button type="button" className={primaryActionClass} onClick={() => drawer.open('count-start', { countId: count.countId })}>
                  开始盘点
                </Button>
              ) : null}
            </>
          ) : undefined
        }
        aside={
          count ? (
            <>
              {status === 'PENDING' ? (
                <WmsAsideBlock title="开始盘点会怎样">
                  <span>开始后盘点范围里的库存进入作业锁定，到结束或作废才放开。</span>
                  {drawer.has('count-start') ? null : (
                    <span className="type-caption text-[color:var(--ds-status-warning-fg)]">{hiddenDecisionHint('开始、结束与作废', '库存盘点')}</span>
                  )}
                </WmsAsideBlock>
              ) : null}
              {status === 'COUNTING' ? (
                <WmsAsideBlock title="结束盘点会怎样">
                  <span>每个库位都要有清点录入才能结束。结束时按每个库位最后一次的清点数与账面比对，不一致的各成一条差异，等批准或驳回。</span>
                  {drawer.has('count-complete') ? null : (
                    <span className="type-caption text-[color:var(--ds-status-warning-fg)]">{hiddenDecisionHint('开始、结束与作废', '库存盘点')}</span>
                  )}
                </WmsAsideBlock>
              ) : null}
              {status === 'COUNTED' && totals.pendingRows > 0 ? (
                <WmsAsideBlock title="批准会怎样">
                  <span>批准后按清点数调整库存账：盘盈增加库存，盘亏扣减库存；驳回不改账，这条差异就此结束。</span>
                  <span className={mutedText}>清点人能否自己批准，由租户登记的批准职责规则决定。</span>
                  {mayDecide ? null : (
                    <span className="type-caption text-[color:var(--ds-status-warning-fg)]">{hiddenDecisionHint('批准与驳回', '库存调整批准')}</span>
                  )}
                </WmsAsideBlock>
              ) : null}
              <WmsAsideBlock title={`盘点范围（${count.locations.length} 个库位）`}>
                <div className="flex max-h-64 flex-col gap-1 overflow-y-auto">
                  {count.locations.map((location) => (
                    <div key={location} className="flex items-baseline justify-between gap-2">
                      <span className="font-mono">{location}</span>
                      {status === 'PENDING' ? null : countedLocations.has(location) ? (
                        <span className={mutedText}>已清点</span>
                      ) : (
                        <span className="type-caption text-[color:var(--ds-status-warning-fg)]">{status === 'VOID' ? '没录入' : '还没清点'}</span>
                      )}
                    </div>
                  ))}
                </div>
              </WmsAsideBlock>
            </>
          ) : undefined
        }
      >
        {count ? (
          <>
            {kpis.length > 0 ? <WmsKpiStrip items={kpis} /> : null}
            <WmsInfoGrid
              items={[
                { label: '仓库', value: count.warehouseCode, mono: true },
                { label: '货主范围', value: count.ownerAccount || '不限货主', mono: true },
                { label: '盘点库位', value: codesPreview(count.locations, 6), mono: true },
                { label: '商品范围', value: codesPreview(count.goods, 6) || '范围内全部商品', mono: true },
                { label: '创建', value: `${count.operator} · ${formatInstant(count.createdAt)}` },
                ...(count.startedAt ? [{ label: '开始', value: `${count.startedBy} · ${formatInstant(count.startedAt)}` }] : []),
                ...(status === 'COUNTED' && count.completedAt
                  ? [{ label: '结束', value: `${count.completedBy} · ${formatInstant(count.completedAt)}` }]
                  : []),
                ...(status === 'VOID'
                  ? [
                      { label: '作废', value: `${count.voidedBy} · ${count.voidedAt ? formatInstant(count.voidedAt) : '—'}` },
                      { label: '作废原因', value: count.voidReason },
                    ]
                  : []),
                { label: '备注', value: count.note || '—' },
              ]}
            />
            {status === 'COUNTED' ? (
              <WmsSubTable
                title="差异"
                count={totals.rows}
                extra={totals.rows > 0 ? <span className="type-caption text-idpxyz-textMuted">待批的在前 · {differenceSummary(totals)}</span> : undefined}
                emptyText="账实一致：每个库位的清点数都与账面相符，没有差异。"
                rows={differences}
                rowKey={(row) => row.differenceId}
                columns={differenceColumns}
              />
            ) : null}
            {status === 'COUNTING' || entryRows.length > 0 ? (
              <WmsSubTable
                title="清点录入"
                count={entries.length}
                extra={supersededEntries > 0 ? <span className="type-caption text-idpxyz-textMuted">每个库位以最后一次为准，划掉的是被重录取代的</span> : undefined}
                emptyText="还没有清点录入。到作业台按库位录入清点数。"
                rows={entryRows}
                rowKey={(row) => row.key}
                columns={entryColumns}
              />
            ) : null}
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={reload} />
    </>
  );
}
