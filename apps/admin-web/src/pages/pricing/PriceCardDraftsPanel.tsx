import { Fragment, useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import { ReloginButton } from '../../auth/ReloginButton';
import { moduleInfoById } from '../../navigation';
import { StateSlot } from '../../templates';
import { formatInstant } from '../catalogue-view';
import {
  approvePriceCardDraft,
  listPriceCardDrafts,
  priceCardDraftApprovalPath,
  priceCardDraftPublicationPath,
  priceCardDraftViewsPath,
  publishPriceCardDraft,
  type ApiResult,
  type PriceCardDraftListResponseBody,
  type PriceCardDraftRow,
} from './api';
import { ProblemTable } from './PriceCardImportPanel';
import {
  draftActionNoteOf,
  draftActionOf,
  draftKey,
  draftListViewState,
  draftReferencePayload,
  priceCardDraftActionLabels,
  priceCardDraftStatusLabels,
  sortPriceCardProblems,
  withProgressedDraft,
  type DraftActionNote,
  type PriceCardDraftAction,
} from './price-card-import';
import { labelOf } from './presentation';

const info = moduleInfoById['price-card-catalog'];

const columnCount = 7;

const noteClass: Record<DraftActionNote['tone'], string> = {
  answered: 'text-idpxyz-text',
  unconfigured: 'text-idpxyz-textMuted',
  refused: 'text-idpxyz-textMuted',
  problem: 'text-idpxyz-danger',
};

/**
 * 价卡草稿（票 price-card-import/06；ADR-0101 决定五）：草稿册一版一行，录入者与批准者并排；`草稿`行可展开逐格问题，
 * 已校验的那一版给「批准」、已批准的给「发布」，别的状态不给动作。
 *
 * 批准门（审批职责规则、主体相同、授予不足）全在服务端：页面不预判谁能批，按钮按状态给，答回来哪一格就如实呈现哪一格。
 * 端点今天在部署没设发行方参数时答 403，页面只呈现未配置，不拿演示数据顶替。
 */
export function PriceCardDraftsPanel() {
  const [answer, setAnswer] = useState<ApiResult<PriceCardDraftListResponseBody> | null>(null);
  const [rows, setRows] = useState<PriceCardDraftRow[]>([]);
  const [reloadToken, setReloadToken] = useState(0);
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
  const [pending, setPending] = useState<string | null>(null);
  const [notes, setNotes] = useState<Record<string, DraftActionNote>>({});

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listPriceCardDrafts().then((result) => {
      if (cancelled) return;
      setAnswer(result);
      setRows(result.kind === 'outcome' ? result.body.drafts : []);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  // 刷新后各行按册上重读的样子摆，上一轮动作的答复随之作废，不挂到可能已经换了内容的行上。
  const reload = () => {
    setNotes({});
    setReloadToken((token) => token + 1);
  };

  function toggle(key: string) {
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  async function act(row: PriceCardDraftRow, action: PriceCardDraftAction) {
    const key = draftKey(row.plan);
    setPending(key);
    const payload = draftReferencePayload(row.plan);
    const result = action === 'approve' ? await approvePriceCardDraft(payload) : await publishPriceCardDraft(payload);
    setRows((current) => withProgressedDraft(current, result));
    setNotes((current) => ({ ...current, [key]: draftActionNoteOf(action, result) }));
    setPending(null);
  }

  const viewState = draftListViewState(answer, reload, info);

  return (
    <div className="flex-1 overflow-auto p-4">
      <Card>
        <CardHeader>
          <CardTitle>价卡草稿</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-xs text-idpxyz-textMuted">
            草稿册按 <span className="font-mono">GET {priceCardDraftViewsPath}</span> 读。已校验的那一版可以批准（
            <span className="font-mono">{priceCardDraftApprovalPath}</span>），已批准的可以发布（
            <span className="font-mono">{priceCardDraftPublicationPath}</span>）；两口的载荷只有方案标识与版本，租户与批准者在
            操作者信封里。批准者须不须是另一人、须持哪一格授予，由本租户的价卡发布审批职责规则定，页面不代判。
          </p>
          <div className="flex items-center gap-3">
            <Button variant="outline" onClick={reload} disabled={answer === null || pending !== null}>
              刷新
            </Button>
            {answer?.kind === 'outcome' ? <span className="text-xs text-idpxyz-textMuted">共 {rows.length} 版</span> : null}
          </div>
          {viewState.kind === 'ready' ? (
            <table className="w-full text-left text-xs">
              <thead className="text-idpxyz-textMuted">
                <tr>
                  <th className="py-1 pr-3 font-normal">定价方案</th>
                  <th className="py-1 pr-3 font-normal">状态</th>
                  <th className="py-1 pr-3 font-normal">源文件身份</th>
                  <th className="py-1 pr-3 font-normal">内容摘要</th>
                  <th className="py-1 pr-3 font-normal">录入者</th>
                  <th className="py-1 pr-3 font-normal">批准者</th>
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
                        pending={pending === key}
                        locked={pending !== null}
                        onToggle={() => toggle(key)}
                        onAct={(action) => void act(row, action)}
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
                            <p className={noteClass[note.tone]}>
                              {note.text}
                              {note.tone === 'refused' && note.kind === 'loginRequired' ? (
                                <ReloginButton size="inline" />
                              ) : null}
                            </p>
                          </td>
                        </tr>
                      ) : null}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          ) : (
            <div className="flex justify-center py-8">
              <StateSlot state={viewState} />
            </div>
          )}
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
  onToggle,
  onAct,
}: {
  row: PriceCardDraftRow;
  open: boolean;
  pending: boolean;
  locked: boolean;
  onToggle: () => void;
  onAct: (action: PriceCardDraftAction) => void;
}) {
  const action = draftActionOf(row.status);
  const problems = row.problems ?? [];
  return (
    <tr className="border-t border-idpxyz-border align-top">
      <td className="py-1.5 pr-3">
        <div className="font-mono text-idpxyz-accent">{row.plan.id}</div>
        <div className="font-mono text-[11px] text-idpxyz-textMuted">{row.plan.version}</div>
      </td>
      <td className="py-1.5 pr-3">
        <div>{labelOf(priceCardDraftStatusLabels, row.status)}</div>
        <div className="font-mono text-[11px] text-idpxyz-textMuted">{row.status}</div>
        {row.publishedAt ? (
          <div className="text-[11px] text-idpxyz-textMuted">发布于 {formatInstant(row.publishedAt)}</div>
        ) : null}
      </td>
      <td className="py-1.5 pr-3">
        <div>{row.sourceFile.name}</div>
        <div className="font-mono text-[11px] text-idpxyz-textMuted" title={row.sourceFile.sha256}>
          {row.sourceFile.sha256.slice(0, 12)}…
        </div>
      </td>
      <td className="py-1.5 pr-3">
        {row.content ? (
          <>
            <div className="font-mono" title={row.content.contentDigest}>
              {row.content.contentDigest.slice(0, 16)}…
            </div>
            <div className="font-mono text-[11px] text-idpxyz-textMuted">{row.content.canonicalization}</div>
          </>
        ) : (
          <span className="text-idpxyz-textMuted">—（没过构造门，没有摘要）</span>
        )}
      </td>
      <td className="py-1.5 pr-3">
        <div className="font-mono break-all">{row.submitter}</div>
        <div className="text-[11px] text-idpxyz-textMuted">{formatInstant(row.submittedAt)}</div>
      </td>
      <td className="py-1.5 pr-3">
        {row.approver ? (
          <>
            <div className="font-mono break-all">{row.approver}</div>
            {row.approvedAt ? (
              <div className="text-[11px] text-idpxyz-textMuted">{formatInstant(row.approvedAt)}</div>
            ) : null}
          </>
        ) : (
          <span className="text-idpxyz-textMuted">—（未批准）</span>
        )}
      </td>
      <td className="py-1.5">
        {row.status === 'DRAFT' ? (
          <button type="button" className="text-idpxyz-accent underline" onClick={onToggle}>
            {open ? '收起逐格问题' : `展开逐格问题（${problems.length} 条）`}
          </button>
        ) : null}
        {action ? (
          <Button size="sm" onClick={() => onAct(action)} disabled={locked}>
            {pending ? `${priceCardDraftActionLabels[action]}中…` : priceCardDraftActionLabels[action]}
          </Button>
        ) : null}
      </td>
    </tr>
  );
}
