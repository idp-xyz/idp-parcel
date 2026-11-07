import { Fragment, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle } from '@idpxyz/ui-primitives';
import { ReloginButton } from '../../auth/ReloginButton';
import { moduleInfoById } from '../../navigation';
import { useRetryWhenChanged } from '../../query/answers';
import { StateSlot } from '../../templates';
import { formatInstant } from '../catalogue-view';
import { CursorPager } from '../shipment-request/cursor-pager';
import { useCatalogueCursor } from '../use-catalogue-cursor';
import {
  approvePriceCardDraft,
  listPriceCardDrafts,
  publishPriceCardDraft,
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

interface DraftOverlay {
  source: PriceCardDraftListResponseBody;
  rows: PriceCardDraftRow[];
  notes: Record<string, DraftActionNote>;
}

/**
 * 价卡草稿（票 price-card-import/06；ADR-0101 决定五）：草稿册一版一行，录入者与批准者并排；`草稿`行可展开逐格问题，
 * 已校验的那一版给「批准」、已批准的给「发布」，别的状态不给动作。
 *
 * 批准门（审批职责规则、主体相同、授予不足）全在服务端：页面不预判谁能批，按钮按状态给，答回来哪一格就如实呈现哪一格。
 * 端点今天在部署没设发行方参数时答 403，页面只呈现未配置，不拿演示数据顶替。
 *
 * 草稿查阅已迁到 ADR-0144，按游标翻页（表下翻页条），次序是读口的缺省序（最近录入在前）。读口另有草稿状态一维与 q，本签没有
 * 对应的筛选条，不另加。
 */
export function PriceCardDraftsPanel({ refreshSignal = 0 }: { refreshSignal?: number }) {
  // 没有登记册写授予的操作者只看不摆批准 / 发布：写口照旧自己判授予，这里只是呈现（ADR-0315）。
  const mayWrite = useMayWriteRegistry();
  const { answer, pagination, retry } = useCatalogueCursor({}, listPriceCardDrafts);
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
  const [pending, setPending] = useState<string | null>(null);
  // 批准、发布答回的行与那一口的答复只叠在取到它们的那一份答复上：刷新、翻页都是重读，各行按册上重读的样子摆，上一轮动作的
  // 答复随之作废，不挂到可能已经换了内容的行上。
  const [overlay, setOverlay] = useState<DraftOverlay | null>(null);
  // 别的签（新建价卡）存下了新草稿时拧这把钥匙；挂载时那一次由取数自己取。
  useRetryWhenChanged(refreshSignal, retry);

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const overlaid = body !== null && overlay?.source === body ? overlay : null;
  const rows = overlaid?.rows ?? body?.drafts ?? [];
  const notes = overlaid?.notes ?? {};

  function toggle(key: string) {
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  async function act(row: PriceCardDraftRow, action: PriceCardDraftAction) {
    if (body === null) return;
    const source = body;
    const key = draftKey(row.plan);
    setPending(key);
    const payload = draftReferencePayload(row.plan);
    const result = action === 'approve' ? await approvePriceCardDraft(payload) : await publishPriceCardDraft(payload);
    setOverlay((previous) => {
      const base = previous?.source === source ? previous : { source, rows: source.drafts, notes: {} };
      return {
        source,
        rows: withProgressedDraft(base.rows, result),
        notes: { ...base.notes, [key]: draftActionNoteOf(action, result) },
      };
    });
    setPending(null);
  }

  const viewState = draftListViewState(answer, retry, info);

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
            谁可以批准、录入人能不能自己批准，按贵公司的审批规则。
          </p>
          <div className="flex items-center gap-3">
            <Button variant="outline" onClick={retry} disabled={answer === null || pending !== null}>
              刷新
            </Button>
            {body !== null ? <span className="type-body leading-4 text-idpxyz-textMuted">本页 {rows.length} 个版本</span> : null}
          </div>
          {viewState.kind === 'ready' ? (
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
                        pending={pending === key}
                        locked={pending !== null}
                        mayWrite={mayWrite}
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
                            {note.tone === 'answered' || note.tone === 'problem' ? (
                              <TechnicalDetails rows={note.technical} className="mt-1" />
                            ) : null}
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
  pending: boolean;
  locked: boolean;
  mayWrite: boolean;
  onToggle: () => void;
  onAct: (action: PriceCardDraftAction) => void;
}) {
  const action = draftActionOf(row.status);
  const problems = row.problems ?? [];
  return (
    <tr className="border-t border-idpxyz-border align-top">
      <td className="py-1.5 pr-3">
        <div className="font-mono text-idpxyz-textBright">{row.plan.id}</div>
        <div className="type-caption text-idpxyz-textMuted">版本 {row.plan.version}</div>
      </td>
      <td className="py-1.5 pr-3">
        {/* 草稿四态的词取 priceCardDraftStatusLabels 那一份（编辑器预览也用它），色调按词反查词典。 */}
        <StatusPill label={priceCardDraftStatusLabels[row.status] ?? row.status} code={row.status} />
        {row.publishedAt ? (
          <div className="type-caption text-idpxyz-textMuted">发布于 {formatInstant(row.publishedAt)}</div>
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
        {action && mayWrite ? (
          <Button size="sm" onClick={() => onAct(action)} disabled={locked}>
            {pending ? `${priceCardDraftActionLabels[action]}中…` : priceCardDraftActionLabels[action]}
          </Button>
        ) : null}
      </td>
    </tr>
  );
}
