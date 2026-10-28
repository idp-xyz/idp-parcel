import { useEffect, useState } from 'react';
import { CheckCircle2, XCircle } from 'lucide-react';
import { ConfirmDialog } from '@idpxyz/ui-primitives';
import { ReviewFlowTemplate, type AuditEntry, type ReviewDecisionOption } from '../../templates';
import { ReloginButton } from '../../auth/ReloginButton';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { moduleInfoById } from '../../navigation';
import { labelFor } from '../../vocabulary/status';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  completeManualReview,
  findAcceptanceReviewCase,
  listAcceptanceReviewQueue,
  rejectShipmentRequest,
  type AcceptanceReviewCaseResponseBody,
  type AcceptanceReviewQueueEntry,
  type AcceptanceReviewQueueListResponseBody,
  type ApiResult,
  type ReviewStatusRecord,
} from './api';
import { recordedJudgmentsBlock } from './recorded-judgments';
import { manualReviewCompletionDraftOf } from './manual-review-completion';
import { caseBusinessFields, caseFailureText, caseTechnicalRows } from './review-case';
import { shipmentNoOf } from './ref-no';
import { activeRejectionOutcomeText, commandNoteOf, manualReviewOutcomeText, type OperatorNote } from './words';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';

// 页面标题取导航登记的那一份;主责上下文(parcel-shipment)只在代码里出现,屏上不说系统的话。
const info = moduleInfoById['acceptance-review'];

/**
 * 复核语境下的两个命令 id。
 *
 * 它们不是同一件事的两种表决。CONTEXT.md 明写「复核完成本身不形成决定,决定仍由判断
 * 任务按适用规则形成」——`complete` 落的是留痕,链要不要往前走由下一轮判断说了算
 * (ADR-0086:完成只是把门闩拉开,续办由「复核已完成」信封另行驱动);而 `reject`
 * 是主动拒绝,在自己的命令事务里当场形成决定,不发续办信封(ADR-0086 Decision 三)。
 *
 * 所以按钮不叫「复核通过／复核不通过」:那对标签会让复核角色以为自己在同一个动作的
 * 两个方向上表决,而实际上一个是留痕、另一个是拍板,走的是两条完全不同的路径。
 */
type ReviewCommandId = 'complete' | 'reject';

const reviewCommands: ReviewDecisionOption<ReviewCommandId>[] = [
  { id: 'complete', label: '记录复核完成', variant: 'default', icon: <CheckCircle2 size={13} /> },
  { id: 'reject', label: '拒绝这票运单', variant: 'danger', icon: <XCircle size={13} /> },
];

/** 待确认的「拒绝这票运单」:模板在 onDecide 时已把理由交出并清空输入框,这里把它和目标行一起攥住到确认或取消。 */
interface PendingRejection {
  entry: AcceptanceReviewQueueEntry;
  reason: string;
}

// 确认弹层的正文(票 admin-web-workspace-form/05 第 3 条,蓝图 21.2):写明拒的是哪票运单、影响谁、按什么理由,不写「确定吗」。
// 「主动拒绝」在自己的命令事务里当场形成决定、不发续办信封(ADR-0086 Decision 三)——正文照这条说,与「记录复核完成」分开。
function rejectionConfirmText({ entry, reason }: PendingRejection): string {
  return (
    `将拒绝运单 ${shipmentNoOf(entry.shipmentRequestId)}(客户 ${entry.customerAccountId},${entry.declaredParcelCount} 件)。` +
    `拒绝立即生效,之后不会再变成受理;理由:${reason}。`
  );
}

function reviewStatusText(review: ReviewStatusRecord): string {
  if (review.completed) {
    const at = review.completedAt ? `,${formatInstant(review.completedAt)}` : '';
    return `已记录复核(复核人 ${review.reviewer ?? '—'}${at}),等系统继续判断`;
  }
  return review.waitingOn ? `${labelFor(review.waitingOn, 'resume-path')},还没记录复核` : '不在等待复核';
}

/** 复核留痕进审计区:这是后端事实,不是页面加工出来的展示语义。 */
function auditTrailOf(review: ReviewStatusRecord): AuditEntry[] {
  if (!review.completed) return [];
  return [
    {
      id: 'manual-review-completed',
      title: '复核已完成',
      description: `复核人 ${review.reviewer ?? '—'}${review.evidence ? ` · 依据 ${review.evidence}` : ''}`,
      timestamp: review.completedAt ? formatInstant(review.completedAt) : undefined,
      variant: 'success',
    },
  ];
}

function queueStatusOf(entry: AcceptanceReviewQueueEntry) {
  return (
    <span className="shrink-0 text-[11px] text-idpxyz-textMuted">{entry.reviewCompleted ? '已复核,等系统继续' : '等待复核'}</span>
  );
}

function NoteLine({ note }: { note: OperatorNote | null }) {
  if (!note) return null;
  return (
    <div className={`text-[12px] ${note.tone === 'outcome' ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'}`}>
      {note.text}
      {note.tone === 'refused' && note.kind === 'loginRequired' ? <ReloginButton size="inline" /> : null}
      <TechnicalDetails rows={note.technical} />
    </div>
  );
}

/**
 * 接受前人工复核。队列语义取 parcel-shipment CONTEXT.md 的「等待人工复核」态:
 * 只有适用规则显式要求人工业务判断时,接受判断任务才进入该态,续办方是授权复核角色。
 *
 * 本页接 GET /acceptance-review-queue 的列表与单份两支,决定按钮打两个命令端点
 * (票 admin-skeleton-closure-batch/09)。命令面今天必答 403——那是 ADR-0055 的设计,
 * 页面如实呈现,不用开发用采信身份把它绕过去。
 */
export function AcceptanceReviewPage() {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [queueAnswer, setQueueAnswer] = useState<ApiResult<AcceptanceReviewQueueListResponseBody> | null>(null);
  const [caseLoaded, setCaseLoaded] = useState<{ id: string; answer: ApiResult<AcceptanceReviewCaseResponseBody> } | null>(null);
  const [commandPending, setCommandPending] = useState(false);
  const [commandNote, setCommandNote] = useState<OperatorNote | null>(null);
  const [pendingRejection, setPendingRejection] = useState<PendingRejection | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listAcceptanceReviewQueue().then((answer) => {
      if (!cancelled) setQueueAnswer(answer);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  useEffect(() => {
    if (!selectedId) {
      setCaseLoaded(null);
      return;
    }
    let cancelled = false;
    void findAcceptanceReviewCase(selectedId).then((answer) => {
      if (!cancelled) setCaseLoaded({ id: selectedId, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [selectedId, reloadKey]);

  const queueBody = queueAnswer?.kind === 'outcome' ? queueAnswer.body : null;
  const entries = queueBody?.entries ?? [];
  const caseAnswer = caseLoaded?.id === selectedId ? caseLoaded.answer : null;
  const caseBody = caseAnswer?.kind === 'outcome' ? caseAnswer.body : null;
  const retry = () => setReloadKey((value) => value + 1);

  // 命令成立才重读:403 之后队列没有任何变化,重读只会白跑一趟。
  const settle = (note: OperatorNote, reload: boolean) => {
    setCommandPending(false);
    setCommandNote(note);
    if (reload) retry();
  };

  // 两个命令各自 await 自己那一支:两支的响应体是不同的封闭形状,合成一个三元表达式
  // 会把它们并成联合类型,于是谁的 outcome 词表都对不上。
  const decide = (command: ReviewCommandId, entry: AcceptanceReviewQueueEntry, reason: string) => {
    setCommandPending(true);
    setCommandNote(null);
    if (command === 'complete') {
      void completeManualReview(manualReviewCompletionDraftOf(entry, reason)).then((answer) => {
        settle(commandNoteOf('记录复核完成', answer, manualReviewOutcomeText, (body) => body.outcome), answer.kind === 'outcome');
      });
      return;
    }
    void rejectShipmentRequest({ shipmentRequestId: entry.shipmentRequestId, reason }).then((answer) => {
      settle(commandNoteOf('拒绝这票运单', answer, activeRejectionOutcomeText, (body) => body.outcome), answer.kind === 'outcome');
    });
  };

  // 「拒绝这票运单」按下之后再拦一道确认;「记录复核完成」只落留痕、决定由下一轮判断形成,不拦。
  // 模板交出理由的同时已清空输入框,所以取消确认时要说一声理由需重填,不让人以为它还在。
  const onDecide = ({ itemId, decision, reason }: { itemId: string; decision: ReviewCommandId; reason: string }) => {
    const entry = entries.find((candidate) => candidate.shipmentRequestId === itemId);
    if (!entry) return;
    if (decision === 'reject') {
      setCommandNote(null);
      setPendingRejection({ entry, reason });
      return;
    }
    decide(decision, entry, reason);
  };
  const confirmRejection = () => {
    if (!pendingRejection) return;
    setPendingRejection(null);
    decide('reject', pendingRejection.entry, pendingRejection.reason);
  };
  const cancelRejection = () => {
    setPendingRejection(null);
    setCommandNote({ tone: 'problem', text: '没有拒绝。运单仍在等待复核;理由已清空,要拒绝请重填。', technical: [] });
  };

  return (
    <>
      <ReviewFlowTemplate<ReviewCommandId>
        title={info.title}
        description="规则要求人工看一眼才能受理的运单停在这里。记录复核完成后由系统接着判断能不能受理;确定不能受理的可以直接拒绝。"
        queueTitle="等待复核的运单"
        queue={entries.map((entry) => ({
          id: entry.shipmentRequestId,
          title: shipmentNoOf(entry.shipmentRequestId),
          subtitle: `客户 ${entry.customerAccountId} · ${entry.declaredParcelCount} 件`,
          status: queueStatusOf(entry),
          meta: formatInstant(entry.submittedAt),
        }))}
        selectedId={selectedId}
        onSelect={setSelectedId}
        detailTitle="复核这票运单"
        detailFields={caseBody ? [...caseBusinessFields(caseBody.request), { label: '复核', value: reviewStatusText(caseBody.review) }] : undefined}
        detailExtra={
          <div className="flex flex-col gap-3">
            {/* 单份读不回时不留空白:详情是复核的依据,缺了要说清缺在哪一步。 */}
            {selectedId && !caseBody && (
              <p className="text-[12px] text-idpxyz-textMuted">{caseAnswer ? caseFailureText(caseAnswer) : '正在读取详情…'}</p>
            )}
            {caseBody && recordedJudgmentsBlock(caseBody.recordedJudgments)}
            <NoteLine note={commandNote} />
            {caseBody && (
              <TechnicalDetails
                rows={caseTechnicalRows(caseBody.request, [
                  { label: '复核停点', value: caseBody.review.waitingOn ?? '' },
                  { label: '复核授权', value: caseBody.review.authority ?? '' },
                  { label: '复核证据', value: caseBody.review.evidence ?? '' },
                ])}
              />
            )}
          </div>
        }
        decisions={reviewCommands}
        decisionPending={commandPending}
        onDecide={onDecide}
        auditTrail={caseBody ? auditTrailOf(caseBody.review) : undefined}
        viewState={catalogueViewState(queueAnswer, entries.length, retry, {
          module: info,
          endpoint: 'GET /acceptance-review-queue',
          emptyTitle: '没有等待复核的运单',
          emptyDescription: '只有规则要求人工判断的运单才会进这个队列;现在没有积压。',
        })}
      />
      {/* 「拒绝这票运单」当场形成决定、不可逆,按下之后再拦一道;确认即发命令,进行中由模板禁用按钮、结果由上面的 commandNote 呈现。 */}
      <ConfirmDialog
        renderFooter={confirmDialogFooter}
        open={pendingRejection !== null}
        tone="danger"
        title="拒绝这票运单"
        message={pendingRejection ? rejectionConfirmText(pendingRejection) : ''}
        confirmLabel="确认拒绝"
        cancelLabel="先不拒绝"
        onConfirm={confirmRejection}
        onCancel={cancelRejection}
      />
    </>
  );
}
