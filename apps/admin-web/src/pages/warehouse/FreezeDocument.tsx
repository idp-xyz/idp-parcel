import { useEffect, useRef } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { formatInstant } from '../catalogue-view';
import { hiddenDecisionHint } from '../operator-grants';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { findFreeze } from './api';
import { lotLabel } from './count-flow';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { freezeIsCarton, freezeNextStep, freezeQuantityLabel, freezeTrack } from './freeze-flow';
import { freezeKind, processingModule } from './processing-address';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { openDocument, useReturnToList } from './use-document';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper } from './wms-ui';

const moduleId = processingModule;

/**
 * 冻结单页：冻的是什么（哪个库位上的哪一格货，或哪一整箱）、冻了多少、为什么冻、谁冻的、解冻了没有。冻着的数量不能被出库分配，
 * 库内作业也取不走它；解冻是整份解冻，在头区做。库存页没有按库位或商品的地址，所以库位与箱号只写、不做链接。
 *
 * 同一张页也摆在列表的详情栏里（给了 `panel`）：换一行留着上一张，新的读到再换。栏里做完一个动作告诉列表重取，
 * 列表重取会让 `revision` 变，这张单跟着重读。
 */
export function FreezeDocument({
  freezeId,
  panel,
}: {
  freezeId: string;
  panel?: { onClose: () => void; onChanged: () => void; revision: number };
}) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const back = useReturnToList(moduleId);
  const complete = freezeId !== '';
  const { answer, loading, reload, held, reloading } = useKeyedLoad(complete ? freezeId : null, findFreeze, { holdPrevious: panel !== undefined });
  const shownId = useRef(freezeId);
  useEffect(() => {
    if (shownId.current === freezeId) return;
    shownId.current = freezeId;
    drawer.close();
  }, [freezeId, drawer]);
  const revision = panel?.revision;
  const readAt = useRef(revision);
  useEffect(() => {
    if (readAt.current === revision) return;
    readAt.current = revision;
    reload();
  }, [revision, reload]);
  const found = answer?.kind === 'outcome' ? answer.body : undefined;
  const record = found?.record;
  const failure =
    (complete ? null : '地址里缺冻结号：冻结单的地址是「冻结 / 冻结号」两段。') ??
    readFailureMessage(answer) ??
    (found && !record
      ? found.truncated
        ? `按「${freezeId}」检索到的结果多于一页，前 ${found.searched} 条里没有它。请回列表，用检索词找。`
        : `没有 ${freezeId} 这张冻结单。它可能属于别的公司，或冻结号有误。`
      : null);

  const now = new Date();
  const carton = record ? freezeIsCarton(record) : false;
  const track = record ? freezeTrack(record.status) : null;
  const next = record ? freezeNextStep(record.status) : null;
  const canUnfreeze = next !== null && drawer.has('stock-unfreeze');
  // 冻着的从冻结时刻算到现在；已解冻的冻了多久，算到解冻那一刻。
  const frozenFor = !record
    ? ''
    : record.status === 'FROZEN'
      ? elapsedLabel(record.frozenAt, now)
      : record.unfrozenAt
        ? elapsedLabel(record.frozenAt, new Date(record.unfrozenAt))
        : '';

  return (
    <>
      <WmsDocumentPage
        kind="冻结单"
        title={record?.freezeId ?? freezeId}
        subtitle={record ? [`货主 ${record.ownerAccount}`, `仓 ${record.warehouseCode}`, `冻结于 ${formatInstant(record.frozenAt)}`].join(' · ') : undefined}
        badge={record ? <WmsBadge code={record.status} /> : undefined}
        loading={held ? false : loading}
        reloading={reloading}
        busy={held}
        error={failure}
        onBack={back}
        onReload={reload}
        panel={panel ? { onClose: panel.onClose, onOpenPage: () => openDocument(moduleId, [freezeKind, record?.freezeId ?? freezeId]) } : undefined}
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
          record && canUnfreeze ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('stock-unfreeze', { freezeId: record.freezeId })}>
              解冻
            </Button>
          ) : undefined
        }
        aside={
          record ? (
            record.status === 'FROZEN' ? (
              <WmsAsideBlock title="冻结中会怎样">
                <span>冻着的数量不能被出库分配，库内作业（上架、移库、转换）也取不走它。</span>
                <span className="type-caption text-idpxyz-textMuted">解冻是整份解冻，不能只解一部分；解冻后数量回到可用量。</span>
                {canUnfreeze ? null : <span className="type-caption text-[color:var(--ds-status-warning-fg)]">{hiddenDecisionHint('解冻', '库存冻结')}</span>}
              </WmsAsideBlock>
            ) : record.status === 'UNFROZEN' ? (
              <WmsAsideBlock title="已解冻">
                <span>整份已经解冻，数量回到可用量。</span>
                <span className="type-caption text-idpxyz-textMuted">这份冻结留着作记录；要再冻是新的一份冻结。</span>
              </WmsAsideBlock>
            ) : undefined
          ) : undefined
        }
      >
        {record ? (
          <>
            <section className="space-y-2">
              <h3 className="type-body font-semibold text-idpxyz-textBright">原因</h3>
              <p className="whitespace-pre-wrap rounded-lg border border-idpxyz-border bg-idpxyz-sidebar px-4 py-3 type-body text-idpxyz-textBright">
                {record.reason || '冻结时没有写原因。'}
              </p>
            </section>
            <WmsKpiStrip
              items={[
                { label: '冻结数量', value: record.quantity, note: record.unit || (carton ? '箱' : undefined) },
                { label: '对象', value: labelOf(record.target) },
                record.status === 'FROZEN'
                  ? { label: '已冻', value: frozenFor || '刚冻', note: `冻结于 ${formatInstant(record.frozenAt)}`, tone: 'warning' }
                  : { label: '冻结时长', value: frozenFor || '—', note: record.unfrozenAt ? `解冻于 ${formatInstant(record.unfrozenAt)}` : undefined },
              ]}
            />
            <section className="space-y-2">
              <h3 className="type-body font-semibold text-idpxyz-textBright">冻的是什么</h3>
              <WmsInfoGrid
                items={
                  carton
                    ? [
                        { label: '对象', value: labelOf(record.target) },
                        { label: '箱号', value: record.cartonNumber || '—', mono: true },
                        { label: '数量', value: freezeQuantityLabel(record), mono: true },
                      ]
                    : [
                        { label: '对象', value: labelOf(record.target) },
                        { label: '库位', value: record.locationCode || '—', mono: true },
                        { label: '商品', value: record.goodsCode ? <DocumentLink moduleId="warehouse-goods" parts={[record.ownerAccount, record.goodsCode]} /> : '—' },
                        { label: '库存状态', value: record.stockState ? <WmsBadge code={record.stockState} /> : '—' },
                        { label: '单位', value: record.unit || '—' },
                        { label: '数量', value: freezeQuantityLabel(record), mono: true },
                        { label: '批次', value: lotLabel(record) || '—' },
                        { label: '入库批次', value: record.inboundBatchId || '—', mono: true },
                      ]
                }
              />
            </section>
            <WmsInfoGrid
              items={[
                { label: '货主', value: record.ownerAccount, mono: true },
                { label: '仓库', value: record.warehouseCode, mono: true },
                { label: '冻结人', value: record.operator, mono: true },
                { label: '冻结时刻', value: formatInstant(record.frozenAt), mono: true },
                ...(record.status === 'UNFROZEN'
                  ? [
                      { label: '解冻人', value: record.unfreezeOperator || '—', mono: true },
                      { label: '解冻时刻', value: record.unfrozenAt ? formatInstant(record.unfrozenAt) : '—', mono: true },
                      { label: '解冻说明', value: record.unfreezeNote || '—' },
                    ]
                  : []),
              ]}
            />
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={panel ? panel.onChanged : reload} />
    </>
  );
}
