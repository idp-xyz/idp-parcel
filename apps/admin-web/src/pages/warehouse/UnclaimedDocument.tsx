import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { formatInstant } from '../catalogue-view';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { getUnclaimedArrivalDetail, type UnclaimedArrivalRecord } from './api';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { claimDeadline, claimPrefill, claimWindowElapsed, unclaimedNextStep, unclaimedTrack } from './unclaimed-next-step';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { useDisposalRules } from './use-disposal-rules';
import { useReturnToList } from './use-document';
import { durationLabel, elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable, type WmsTone } from './wms-ui';

const moduleId = 'warehouse-unclaimed';

/**
 * 待认领到货页：一件没人认的到货，现场怎么观察的、有什么线索、谁认领过又撤销过、最后怎么处置的。
 * 认领与撤销认领、按规则处置都在头区，按它此刻的状态摆；货被认领后要去作业台收货入账。
 */
export function UnclaimedDocument({ arrivalId }: { arrivalId: string }) {
  const drawer = useActionDrawer(receivingActionSpecs);
  const back = useReturnToList(moduleId);
  const rules = useDisposalRules(0);
  const { answer, loading, reload } = useKeyedLoad(arrivalId, getUnclaimedArrivalDetail);
  const arrival: UnclaimedArrivalRecord | undefined = answer?.kind === 'outcome' ? answer.body.arrival : undefined;
  const failure =
    readFailureMessage(answer) ?? (answer?.kind === 'outcome' && !arrival ? `没有 ${arrivalId} 这件待认领到货。它可能属于别的公司，或到货号有误。` : null);

  const now = new Date();
  const status = arrival?.status;
  // 规则读全了才敢说哪个仓没登记；没读全（null）时只当不知道，下一步按「认领」走。
  const rule = arrival ? rules?.get(arrival.warehouseCode) : undefined;
  const next = arrival ? unclaimedNextStep(arrival, rule, now) : null;
  const track = arrival ? unclaimedTrack(arrival) : null;
  const windowElapsed = arrival && rule ? claimWindowElapsed(arrival.occurredAt, rule.claimWindowSeconds, now) : false;
  const deadline = arrival && rule ? claimDeadline(arrival.occurredAt, rule.claimWindowSeconds) : null;

  const claims = arrival?.claims ?? [];
  const references = arrival?.postingReferences ?? [];
  const revoked = claims.filter((entry) => entry.revocation !== undefined).length;
  // 生效的认领是最近一笔没被撤销的；到货在已认领状态时它一定在。
  const activeClaim = [...claims].reverse().find((entry) => entry.revocation === undefined)?.claim;
  const divertedLines = arrival?.divertedReceipt?.lines ?? [];

  const claimButton =
    arrival && status === 'UNCLAIMED' && drawer.has('unclaimed-claim') ? (
      <Button
        type="button"
        variant={next?.kind === 'claim' ? undefined : 'outline'}
        className={next?.kind === 'claim' ? primaryActionClass : undefined}
        onClick={() => drawer.open('unclaimed-claim', claimPrefill(arrival))}
      >
        认领
      </Button>
    ) : null;
  const disposeButton =
    arrival && status === 'UNCLAIMED' && drawer.has('unclaimed-dispose') ? (
      <Button
        type="button"
        variant={next?.kind === 'dispose' ? undefined : 'outline'}
        className={next?.kind === 'dispose' ? primaryActionClass : undefined}
        onClick={() => drawer.open('unclaimed-dispose', { arrivalId: arrival.arrivalId })}
      >
        按规则处置
      </Button>
    ) : null;
  // 已被入账引用的认领撤不掉（服务端会拒），不摆一个必然失败的按钮；原因写在右栏。
  const revokeButton =
    arrival && status === 'CLAIMED' && references.length === 0 && drawer.has('unclaimed-revoke') ? (
      <Button type="button" variant="outline" onClick={() => drawer.open('unclaimed-revoke', { arrivalId: arrival.arrivalId })}>
        撤销认领
      </Button>
    ) : null;

  const kpis: Array<{ label: string; value: string | number; note?: string; tone?: WmsTone }> = [];
  if (arrival && (status === 'UNCLAIMED' || status === 'CLAIMED')) {
    kpis.push({ label: '已放', value: elapsedLabel(arrival.occurredAt, now) || '刚到', note: `发生于 ${formatInstant(arrival.occurredAt)}` });
  }
  if (arrival && status === 'UNCLAIMED' && rules !== null) {
    kpis.push(
      rule
        ? {
            label: '认领期限',
            value: durationLabel(rule.claimWindowSeconds),
            note: windowElapsed ? '已过，可以按规则处置' : deadline ? `到 ${formatInstant(deadline.toISOString(), { precision: 'minute' })}` : undefined,
            tone: windowElapsed ? 'warning' : undefined,
          }
        : { label: '认领期限', value: '未配置', note: '这个仓没登记处置规则', tone: 'warning' },
    );
  }
  kpis.push({ label: '认领记录', value: claims.length, note: revoked > 0 ? `撤销过 ${revoked} 次` : undefined });
  kpis.push({
    label: '入账引用',
    value: references.length,
    note: references.length > 0 ? '认领已被入账引用，不能撤销' : undefined,
  });

  return (
    <>
      <WmsDocumentPage
        kind="待认领到货"
        measure="件"
        title={arrival?.arrivalId ?? arrivalId}
        subtitle={arrival ? [`仓 ${arrival.warehouseCode}`, labelOf(arrival.kind), `登记于 ${formatInstant(arrival.receivedAt)}`].join(' · ') : undefined}
        badge={
          arrival ? (
            <>
              <WmsBadge code={arrival.status} />
              {status === 'UNCLAIMED' && windowElapsed ? <WmsBadge label="已过认领期" tone="warning" /> : null}
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
          arrival ? (
            <>
              {/* 主动作排在最后：下一步是处置时先摆认领，否则先摆处置。 */}
              {next?.kind === 'dispose' ? (
                <>
                  {claimButton}
                  {disposeButton}
                </>
              ) : (
                <>
                  {disposeButton}
                  {claimButton}
                </>
              )}
              {revokeButton}
              {next?.kind === 'go' ? (
                <Button
                  type="button"
                  className={primaryActionClass}
                  onClick={() => {
                    window.location.hash = `#/${next.moduleId}`;
                  }}
                >
                  {next.label}
                </Button>
              ) : null}
            </>
          ) : undefined
        }
        aside={
          arrival ? (
            <>
              <WmsAsideBlock title="关联单据">
                {activeClaim?.preAdviceId ? (
                  <div className="flex items-baseline gap-2">
                    <span className="shrink-0 text-idpxyz-textMuted">认给预报</span>
                    <DocumentLink moduleId="warehouse-pre-advices" parts={[activeClaim.ownerAccount, activeClaim.preAdviceId]} />
                  </div>
                ) : activeClaim ? (
                  <span className="text-idpxyz-textMuted">认给货主 {activeClaim.ownerAccount}，没有指名入库预报。</span>
                ) : (
                  <span className="text-idpxyz-textMuted">还没有人认领，没有关联的入库预报。</span>
                )}
              </WmsAsideBlock>
              {status === 'UNCLAIMED' && rules !== null ? (
                <WmsAsideBlock title="认领期限与处置规则">
                  {rule ? (
                    <>
                      <span>认领期限 {durationLabel(rule.claimWindowSeconds)}，自到货发生起算。</span>
                      <span className={windowElapsed ? 'text-amber-600 dark:text-amber-400' : 'text-idpxyz-textMuted'}>
                        {windowElapsed ? '期限已过，可以按规则处置。' : deadline ? `到 ${formatInstant(deadline.toISOString())} 为止，期限内只能认领。` : ''}
                      </span>
                      <span className="type-caption text-idpxyz-textMuted">
                        可选的处置方式：{rule.disposalMethods.map((method) => labelOf(method)).join('、') || '—'}
                      </span>
                    </>
                  ) : (
                    <>
                      <span className="text-amber-600 dark:text-amber-400">{arrival.warehouseCode} 仓还没登记处置规则，处置会答未配置。</span>
                      <button
                        type="button"
                        className="self-start text-[color:var(--ds-text-link-hover)] hover:underline"
                        onClick={() => {
                          window.location.hash = '#/warehouse-settings';
                        }}
                      >
                        去仓配规则设置登记
                      </button>
                    </>
                  )}
                </WmsAsideBlock>
              ) : null}
              {status === 'CLAIMED' && references.length > 0 ? (
                <WmsAsideBlock title="认领不能撤销">
                  <span className="text-idpxyz-textMuted">这一认领已经被收货入账引用，货进了库存账，撤销会让账对不上。</span>
                </WmsAsideBlock>
              ) : null}
            </>
          ) : undefined
        }
      >
        {arrival ? (
          <>
            <WmsKpiStrip items={kpis} />
            <section className="space-y-2">
              <h3 className="type-body font-semibold text-idpxyz-textBright">现场观察</h3>
              <p className="whitespace-pre-wrap rounded-lg border border-idpxyz-border bg-idpxyz-panel px-4 py-3 type-body text-idpxyz-textBright">
                {arrival.observation || '登记时没有写现场观察。'}
              </p>
            </section>
            <WmsInfoGrid
              items={[
                { label: '仓库', value: arrival.warehouseCode, mono: true },
                { label: '种类', value: labelOf(arrival.kind) },
                { label: '来源', value: labelOf(arrival.origin) },
                { label: '暂存库位', value: arrival.storageLocation || '—', mono: true },
                { label: '货主线索', value: arrival.ownerClue || '—', mono: true },
                { label: '预报线索', value: arrival.preAdviceClue || '—', mono: true },
                { label: '登记设备', value: arrival.device, mono: true },
                { label: '登记人', value: arrival.operator, mono: true },
                { label: '发生时刻', value: formatInstant(arrival.occurredAt), mono: true },
                { label: '收到时刻', value: formatInstant(arrival.receivedAt), mono: true },
                { label: '事实标识', value: arrival.factId, mono: true },
                ...(references.length > 0 ? [{ label: '入账引用', value: references.join('、'), mono: true }] : []),
              ]}
            />
            {arrival.divertedReceipt ? (
              <WmsSubTable
                title="转来的收货"
                count={divertedLines.length}
                extra={<span className="type-caption text-idpxyz-textMuted">原是对已取消或已完结预报的收货，没有入库</span>}
                emptyText="这次收货没有留下行。"
                rows={divertedLines}
                rowKey={(line, index) => `${line.goodsCode ?? line.cartonNumber ?? line.observed ?? ''}-${index}`}
                columns={[
                  { id: 'what', header: '商品 / 箱', mono: true, render: (line) => line.goodsCode || line.cartonNumber || line.observed || '—' },
                  { id: 'state', header: '库存状态', render: (line) => (line.stockState ? <WmsBadge code={line.stockState} /> : '—') },
                  { id: 'batch', header: '批号', mono: true, render: (line) => line.batchNumber || '—' },
                  { id: 'qty', header: '数量', align: 'right', mono: true, render: (line) => (line.quantity ? `${line.quantity} ${line.unit ?? ''}`.trim() : '—') },
                ]}
              />
            ) : null}
            {claims.length > 0 ? (
              <WmsSubTable
                title="认领记录"
                count={claims.length}
                emptyText=""
                rows={claims}
                rowKey={(entry) => entry.claim.decisionId}
                columns={[
                  { id: 'owner', header: '认领货主', mono: true, render: (entry) => entry.claim.ownerAccount },
                  {
                    id: 'advice',
                    header: '认给预报',
                    render: (entry) =>
                      entry.claim.preAdviceId ? (
                        <DocumentLink moduleId="warehouse-pre-advices" parts={[entry.claim.ownerAccount, entry.claim.preAdviceId]} />
                      ) : (
                        '—'
                      ),
                  },
                  { id: 'who', header: '认领人', mono: true, render: (entry) => entry.claim.operator },
                  { id: 'at', header: '认领时刻', mono: true, render: (entry) => formatInstant(entry.claim.claimedAt) },
                  {
                    id: 'revoked',
                    header: '撤销',
                    render: (entry) =>
                      entry.revocation
                        ? `${entry.revocation.operator} · ${formatInstant(entry.revocation.revokedAt)}${entry.revocation.reason ? ` · ${entry.revocation.reason}` : ''}`
                        : '—',
                  },
                ]}
              />
            ) : null}
            {arrival.disposition ? (
              <section className="space-y-2">
                <h3 className="type-body font-semibold text-idpxyz-textBright">处置</h3>
                <WmsInfoGrid
                  items={[
                    { label: '处置方式', value: labelOf(arrival.disposition.method) },
                    { label: '依据规则', value: `${arrival.disposition.ruleWarehouseCode} r${arrival.disposition.ruleRevision}`, mono: true },
                    { label: '处置人', value: arrival.disposition.operator, mono: true },
                    { label: '处置时刻', value: formatInstant(arrival.disposition.disposedAt), mono: true },
                    { label: '说明', value: arrival.disposition.note || '—' },
                  ]}
                />
              </section>
            ) : null}
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={reload} />
    </>
  );
}
