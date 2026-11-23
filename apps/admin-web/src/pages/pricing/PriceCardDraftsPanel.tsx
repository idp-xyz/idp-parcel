import { Fragment, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import { ReloginButton } from '../../auth/ReloginButton';
import { moduleInfoById } from '../../navigation';
import { useRetryWhenChanged } from '../../query/answers';
import { StateSlot } from '../../templates';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { formatInstant } from '../catalogue-view';
import { CursorPager } from '../shipment-request/cursor-pager';
import { useCatalogueCursor } from '../use-catalogue-cursor';
import {
  approvePriceCardDraft,
  listPriceCardDrafts,
  publishPriceCardDraft,
  withdrawPriceCardDraft,
  type PriceCardDraftListResponseBody,
  type PriceCardDraftRow,
  type PriceCardOverlap,
} from './api';
import { CountChips } from './CountChips';
import { ProblemTable } from './PriceCardImportPanel';
import { PriceCardOverlapNotice } from './PriceCardOverlapNotice';
import {
  draftActionNoteOf,
  draftActionsOf,
  draftKey,
  draftListConditions,
  draftListViewState,
  draftReferencePayload,
  priceCardDraftActionLabels,
  priceCardDraftStatusLabels,
  priceCardDraftStatusOrder,
  sortPriceCardProblems,
  withdrawalReason,
  withProgressedDraft,
  type DraftActionNote,
  type PriceCardDraftAction,
} from './price-card-import';
import { PrincipalRef } from '../../components/PrincipalRef';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { technicalRows } from '../settlement/ops-ui';
import { useMayWriteRegistry } from '../use-operator-grants';

const info = moduleInfoById['price-card-catalog'];

const columnCount = 6;

const noteClass: Record<DraftActionNote['tone'], string> = {
  answered: 'text-idpxyz-text',
  unconfigured: 'text-idpxyz-textMuted',
  refused: 'text-idpxyz-textMuted',
  problem: 'text-idpxyz-danger',
};

/** 一次批准或发布的答复，连同适用期重叠提示。提示不改那一句结果。 */
export function DraftActionFeedback({
  note,
  overlaps,
}: {
  note: DraftActionNote;
  overlaps?: readonly PriceCardOverlap[];
}) {
  return (
    <>
      <p className={noteClass[note.tone]}>
        {note.text}
        {note.tone === 'refused' && note.kind === 'loginRequired' ? <ReloginButton size="inline" /> : null}
      </p>
      {note.tone === 'answered' || note.tone === 'problem' ? <TechnicalDetails rows={note.technical} className="mt-1" /> : null}
      <PriceCardOverlapNotice overlaps={overlaps} />
    </>
  );
}

interface DraftOverlay {
  source: PriceCardDraftListResponseBody;
  rows: PriceCardDraftRow[];
  notes: Record<string, DraftActionNote>;
  overlaps: Record<string, PriceCardOverlap[] | undefined>;
}

/**
 * 价卡草稿（票 price-card-import/06；ADR-0101 决定五）：草稿册一版一行，录入者与批准者并排；`草稿`行可展开逐格问题，
 * 已校验的那一版给「批准」、已批准的给「发布」和「撤下」，别的状态不给动作。撤下要填写原因。
 *
 * 批准门（审批职责规则、主体相同、授予不足）全在服务端：页面不预判谁能批，按钮按状态给，答回来哪一格就如实呈现哪一格。
 * 端点今天在部署没设发行方参数时答 403，页面只呈现未配置，不拿演示数据顶替。
 *
 * 草稿查阅已迁到 ADR-0144，按游标翻页（表下翻页条），次序是读口的缺省序（最近录入在前）。状态胶囊把读口的 status 维下推，
 * 不在答回来的这一页上再筛。
 */
export function PriceCardDraftsPanel({ refreshSignal = 0 }: { refreshSignal?: number }) {
  // 没有登记册写授予的操作者只看不摆批准 / 发布 / 撤下：写口照旧自己判授予，这里只是呈现（ADR-0315）。
  const mayWrite = useMayWriteRegistry();
  const [status, setStatus] = useState<string | null>(null);
  const conditions = draftListConditions(status);
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listPriceCardDrafts);
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
  const [pending, setPending] = useState<{ key: string; action: PriceCardDraftAction } | null>(null);
  // 批准、发布答回的行与那一口的答复只叠在取到它们的那一份答复上：刷新、翻页都是重读，各行按册上重读的样子摆，上一轮动作的
  // 答复随之作废，不挂到可能已经换了内容的行上。
  const [overlay, setOverlay] = useState<DraftOverlay | null>(null);
  // 别的签（新建价卡）存下了新草稿时拧这把钥匙；挂载时那一次由取数自己取。
  useRetryWhenChanged(refreshSignal, retry);

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const overlaid = body !== null && overlay?.source === body ? overlay : null;
  const rows = overlaid?.rows ?? body?.drafts ?? [];
  const notes = overlaid?.notes ?? {};
  const overlapByKey = overlaid?.overlaps ?? {};

  function toggle(key: string) {
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  async function act(row: PriceCardDraftRow, action: PriceCardDraftAction, reason?: string) {
    if (body === null) return;
    const source = body;
    const key = draftKey(row.plan);
    setPending({ key, action });
    const payload = draftReferencePayload(row.plan);
    const result =
      action === 'approve'
        ? await approvePriceCardDraft(payload)
        : action === 'publish'
          ? await publishPriceCardDraft(payload)
          : await withdrawPriceCardDraft({ ...payload, reason: reason ?? '' });
    setOverlay((previous) => {
      const base = previous?.source === source ? previous : { source, rows: source.drafts, notes: {}, overlaps: {} };
      return {
        source,
        rows: withProgressedDraft(base.rows, result),
        notes: { ...base.notes, [key]: draftActionNoteOf(action, result) },
        overlaps: { ...base.overlaps, [key]: result.kind === 'outcome' ? result.body.overlaps : undefined },
      };
    });
    setPending(null);
  }

  const viewState = draftListViewState(answer, retry, info, catalogueConditionsNarrow(conditions));

  return (
    <div className="flex-1 overflow-auto p-4">
      <Card>
        <CardHeader>
          <CardTitle>草稿与审批</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {/* 批准者是谁、须不须另一人由审批职责规则在写口判，页面不预判（ADR-0101 决定五）。 */}
          <p className="type-body leading-4 text-idpxyz-textMuted">
            新建或导入的价卡先存成草稿。检查通过的可以批准，批准后再发布；发布后就出现在「全部价卡」里，开始参与计价。
            已批准、还没发布的可以撤下，要写明原因。撤下之后这一版不能再发布，版本号也不再使用。
            谁可以批准、录入人能不能自己批准，按贵公司的审批规则。撤下不另要一道审批。
          </p>
          <CountChips
            label="状态"
            chips={priceCardDraftStatusOrder.map((code) => ({ id: code, label: priceCardDraftStatusLabels[code] ?? code }))}
            value={status}
            onChange={setStatus}
          />
          <div className="flex items-center gap-3">
            <Button variant="outline" onClick={retry} disabled={answer === null || pending !== null}>
              刷新
            </Button>
            {body !== null ? <span className="type-body leading-4 text-idpxyz-textMuted">本页 {rows.length} 个版本</span> : null}
          </div>
          {viewState.kind === 'ready' ? (
            rows.length === 0 ? (
              <p className="type-body leading-4 text-idpxyz-textMuted">当前状态下没有草稿。</p>
            ) : (
            <table className="w-full text-left type-body leading-4">
              <thead className="text-idpxyz-textMuted">
                <tr>
                  <th className="py-1 pr-3 font-normal">价卡</th>
                  <th className="py-1 pr-3 font-normal">状态</th>
                  <th className="py-1 pr-3 font-normal">来源</th>
                  <th className="py-1 pr-3 font-normal">录入人</th>
                  <th className="py-1 pr-3 font-normal">批准人</th>
                  <th className="py-1 font-normal">操作</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => {
                  const key = draftKey(row.plan);
                  const note = notes[key];
                  return (
                    <Fragment key={key}>
                      <DraftRow
                        row={row}
                        open={expanded.has(key)}
                        pending={pending?.key === key ? pending.action : null}
                        locked={pending !== null}
                        mayWrite={mayWrite}
                        onToggle={() => toggle(key)}
                        onAct={(action, reason) => void act(row, action, reason)}
                      />
                      {row.status === 'DRAFT' && expanded.has(key) ? (
                        <tr>
                          <td colSpan={columnCount} className="pb-2 pl-3">
                            <ProblemTable problems={sortPriceCardProblems(row.problems ?? [])} />
                          </td>
                        </tr>
                      ) : null}
                      {note ? (
                        <tr>
                          <td colSpan={columnCount} className="pb-2">
                            <DraftActionFeedback note={note} overlaps={overlapByKey[key]} />
                          </td>
                        </tr>
                      ) : null}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
            )
          ) : (
            <div className="flex justify-center py-8">
              <StateSlot state={viewState} />
            </div>
          )}
          {pagination && viewState.kind === 'ready' ? <CursorPager pagination={pagination} /> : null}
        </CardContent>
      </Card>
    </div>
  );
}

function DraftRow({
  row,
  open,
  pending,
  locked,
  mayWrite,
  onToggle,
  onAct,
}: {
  row: PriceCardDraftRow;
  open: boolean;
  pending: PriceCardDraftAction | null;
  locked: boolean;
  mayWrite: boolean;
  onToggle: () => void;
  onAct: (action: PriceCardDraftAction, reason?: string) => void;
}) {
  const [reason, setReason] = useState('');
  const actions = draftActionsOf(row.status);
  const cleanedReason = withdrawalReason(reason);
  const problems = row.problems ?? [];
  return (
    <tr className="border-t border-idpxyz-border align-top">
      <td className="py-1.5 pr-3">
        <div className="font-mono text-idpxyz-textBright">{row.plan.id}</div>
        <div className="type-caption text-idpxyz-textMuted">版本 {row.plan.version}</div>
      </td>
      <td className="py-1.5 pr-3">
        {/* 词取 priceCardDraftStatusLabels 那一份（编辑器预览也用它）。WITHDRAWN 在总表里是「已撤回」，这里显「已撤下」。 */}
        <StatusPill label={priceCardDraftStatusLabels[row.status] ?? row.status} code={row.status} />
        {row.publishedAt ? (
          <div className="type-caption text-idpxyz-textMuted">发布于 {formatInstant(row.publishedAt)}</div>
        ) : null}
        {row.status === 'WITHDRAWN' ? (
          <div className="mt-1 max-w-xs type-caption text-idpxyz-textMuted">
            {row.withdrawnBy ? (
              <div>
                撤下人 <PrincipalRef value={row.withdrawnBy} />
              </div>
            ) : null}
            {row.withdrawnAt ? <div>撤下于 {formatInstant(row.withdrawnAt)}</div> : null}
            {row.withdrawalReason ? <div>原因 {row.withdrawalReason}</div> : null}
          </div>
        ) : null}
      </td>
      <td className="py-1.5 pr-3">
        <div>{row.sourceOrigin === 'FORM' ? '页面新建' : row.sourceFile.name}</div>
        {/* 源文件指纹与内容摘要是对账用的比对串；没过检查的草稿没有内容摘要，那一行就不出。 */}
        <TechnicalDetails
          className="mt-0.5 max-w-xs"
          rows={technicalRows([
            { label: '文件名', value: row.sourceFile.name },
            { label: '文件指纹', value: row.sourceFile.sha256 },
            { label: '内容摘要', value: row.content?.contentDigest },
            { label: '规范化版本', value: row.content?.canonicalization },
            { label: '状态码', value: row.status },
            { label: '撤下人', value: row.withdrawnBy },
            { label: '撤下时间', value: row.withdrawnAt },
            { label: '撤下原因', value: row.withdrawalReason },
          ])}
        />
      </td>
      <td className="py-1.5 pr-3">
        <div><PrincipalRef value={row.submitter} /></div>
        <div className="type-caption text-idpxyz-textMuted">{formatInstant(row.submittedAt)}</div>
      </td>
      <td className="py-1.5 pr-3">
        {row.approver ? (
          <>
            <div><PrincipalRef value={row.approver} /></div>
            {row.approvedAt ? (
              <div className="type-caption text-idpxyz-textMuted">{formatInstant(row.approvedAt)}</div>
            ) : null}
          </>
        ) : (
          <span className="text-idpxyz-textMuted">未批准</span>
        )}
      </td>
      <td className="py-1.5">
        {row.status === 'DRAFT' ? (
          <button type="button" className="text-idpxyz-accent underline" onClick={onToggle}>
            {open ? '收起问题' : `查看问题（${problems.length} 条）`}
          </button>
        ) : null}
        {actions.includes('approve') && mayWrite ? (
          <Button size="sm" onClick={() => onAct('approve')} disabled={locked}>
            {pending === 'approve' ? `${priceCardDraftActionLabels.approve}中…` : priceCardDraftActionLabels.approve}
          </Button>
        ) : null}
        {actions.includes('publish') && mayWrite ? (
          <Button size="sm" onClick={() => onAct('publish')} disabled={locked}>
            {pending === 'publish' ? `${priceCardDraftActionLabels.publish}中…` : priceCardDraftActionLabels.publish}
          </Button>
        ) : null}
        {actions.includes('withdraw') && mayWrite ? (
          <form
            className="mt-1 flex max-w-xs flex-col gap-1"
            onSubmit={(event) => {
              event.preventDefault();
              if (cleanedReason === null) return;
              onAct('withdraw', cleanedReason);
            }}
          >
            <label className="type-caption text-idpxyz-textMuted">
              撤下原因
              <textarea
                className="mt-0.5 w-full rounded border border-idpxyz-border bg-transparent px-2 py-1 type-body text-idpxyz-text"
                value={reason}
                required
                rows={2}
                disabled={locked}
                onChange={(event) => setReason(event.target.value)}
              />
            </label>
            <Button size="sm" type="submit" variant="outline" disabled={locked || cleanedReason === null}>
              {pending === 'withdraw' ? `${priceCardDraftActionLabels.withdraw}中…` : priceCardDraftActionLabels.withdraw}
            </Button>
          </form>
        ) : null}
      </td>
    </tr>
  );
}
