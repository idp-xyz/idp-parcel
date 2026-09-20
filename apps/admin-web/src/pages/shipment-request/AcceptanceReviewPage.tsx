import { useEffect, useState } from 'react';
import { CheckCircle2, XCircle } from 'lucide-react';
import { ConfirmDialog } from '@idpxyz/ui-primitives';
import {
  ReviewFlowTemplate,
  type AuditEntry,
  type DetailField,
  type ReviewDecisionOption,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
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

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['acceptance-review'];

/**
 * 复核语境下的两个命令 id。
 *
 * 它们不是同一件事的两种表决。CONTEXT.md 明写「复核完成本身不形成决定，决定仍由判断
 * 任务按适用规则形成」——`complete` 落的是留痕，链要不要往前走由下一轮判断说了算
 * （ADR-0086：完成只是把门闩拉开，续办由「复核已完成」信封另行驱动）；而 `reject`
 * 是主动拒绝，在自己的命令事务里当场形成决定，不发续办信封（ADR-0086 Decision 三）。
 *
 * 所以按钮不叫「复核通过／复核不通过」：那对标签会让复核角色以为自己在同一个动作的
 * 两个方向上表决，而实际上一个是留痕、另一个是拍板，走的是两条完全不同的路径。
 */
type ReviewCommandId = 'complete' | 'reject';

const reviewCommands: ReviewDecisionOption<ReviewCommandId>[] = [
  {
    id: 'complete',
    label: '记录复核完成',
    variant: 'default',
    icon: <CheckCircle2 size={13} />,
  },
  {
    id: 'reject',
    label: '主动拒绝委托',
    variant: 'danger',
    icon: <XCircle size={13} />,
  },
];

/** 命令口的一次答复。渠道未配置是今天的预期答案，不是故障，因此单列一格。 */
type CommandNote =
  | { tone: 'unconfigured'; text: string }
  | { tone: 'outcome'; text: string }
  | { tone: 'problem'; text: string };

function commandNoteOf<Body extends { outcome: string }>(
  label: string,
  answer: ApiResult<Body>,
): CommandNote {
  switch (answer.kind) {
    case 'outcome':
      return { tone: 'outcome', text: `「${label}」结果：${answer.body.outcome}` };
    case 'unconfigured':
      // 命令面一律挂字面量 UnconfiguredIntake（ADR-0055）：隔离读准入换得动读行，换不动
      // 写行。如实说渠道未配置，不退化成「稍后重试」，也不造开发用采信身份把它绕过去。
      return {
        tone: 'unconfigured',
        text: `「${label}」未执行：接入渠道尚未配置（HTTP 403 ACCESS_CHANNEL_NOT_CONFIGURED）。复核人与授权依据要由已认证的操作员身份翻译得出（PAR-INT-01），页面不代签。`,
      };
    case 'callerProblem':
      return { tone: 'problem', text: `「${label}」被拒（HTTP ${answer.status}）：${answer.code}` };
    case 'noAnswer':
      return {
        tone: 'problem',
        text: `「${label}」服务端未形成答案（HTTP ${answer.status}）：${answer.code}`,
      };
    default:
      return { tone: 'problem', text: `「${label}」未送达：${answer.message}` };
  }
}

/** 待确认的「主动拒绝委托」：模板在 onDecide 时已把理由交出并清空输入框，这里把它和目标行一起攥住到确认或取消。 */
interface PendingRejection {
  entry: AcceptanceReviewQueueEntry;
  reason: string;
}

// 确认弹层的正文（票 admin-web-workspace-form/05 第 3 条，蓝图 21.2）：写明拒的是哪份委托、影响谁、按什么理由，不写「确定吗」。
// 「主动拒绝」在自己的命令事务里当场形成决定、不发续办信封（ADR-0086 Decision 三）——正文照这条说，与「记录复核完成」分开。
function rejectionConfirmText({ entry, reason }: PendingRejection): string {
  return (
    `将主动拒绝委托 ${entry.shipmentRequestId}（客户账户 ${entry.customerAccountId}，${entry.declaredParcelCount} 件）。` +
    `拒绝在本次命令里当场形成决定，不再交下一轮判断，形成后不会变成接受；理由：${reason}。`
  );
}

/** 单份读不回时说清缺在哪一步。渠道未配置与「查不到」不并成一句：恢复动作不同。 */
function caseFailureText(answer: ApiResult<AcceptanceReviewCaseResponseBody>): string {
  switch (answer.kind) {
    case 'outcome':
      return '';
    case 'unconfigured':
      return '接入渠道尚未配置';
    case 'transport':
      return answer.message;
    default:
      return `${answer.code}（HTTP ${answer.status}）`;
  }
}

function reviewStatusText(review: ReviewStatusRecord): string {
  if (review.completed) {
    const at = review.completedAt ? `，于 ${formatInstant(review.completedAt)}` : '';
    return `已录完成（复核人 ${review.reviewer ?? '—'} · 授权 ${review.authority ?? '—'}${at}）`;
  }
  return review.waitingOn ? `停在 ${review.waitingOn}，复核未录完成` : '任务不在等待态';
}

function caseDetailFields(body: AcceptanceReviewCaseResponseBody): DetailField[] {
  const request = body.request;
  const fields: DetailField[] = [
    { label: '委托标识', value: <span className="font-mono">{request.shipmentRequestId}</span> },
    { label: '客户账户', value: <span className="font-mono">{request.customerAccountId}</span> },
    {
      label: '来源',
      value: <span className="font-mono">{`${request.source} / ${request.sourceRequestKey}`}</span>,
    },
    { label: '委托状态', value: <span className="font-mono">{request.state}</span> },
    {
      label: '提交版本',
      value: <span className="font-mono">{request.submissionVersionId}</span>,
    },
    { label: '声明包裹', value: `${request.declaredParcelCount} 件` },
    { label: '提交时刻', value: formatInstant(request.submittedAt) },
    { label: '判断任务', value: request.acceptanceTask.state },
    { label: '复核留痕', value: reviewStatusText(body.review) },
  ];
  if (body.review.evidence) {
    fields.push({
      label: '复核证据',
      value: <span className="font-mono">{body.review.evidence}</span>,
    });
  }
  // 最近一次没能推进的处理记录：有过才在场，没有就不占一格（不写「无」冒充记过一次）。
  if (request.acceptanceTask.lastAttemptReason) {
    fields.push({
      label: '最近未推进',
      value: `${request.acceptanceTask.lastAttemptReason}${
        request.acceptanceTask.lastAttemptContinuation
          ? ` · 续办 ${request.acceptanceTask.lastAttemptContinuation}`
          : ''
      }`,
    });
  }
  if (request.decision) {
    fields.push({
      label: '已形成决定',
      value: `${request.decision.kind} · ${formatInstant(request.decision.decidedAt)}`,
    });
  }
  return fields;
}

/** 复核留痕进审计区：这是后端事实，不是页面加工出来的展示语义。 */
function auditTrailOf(review: ReviewStatusRecord): AuditEntry[] {
  if (!review.completed) return [];
  return [
    {
      id: 'manual-review-completed',
      title: '复核已完成',
      description: `复核人 ${review.reviewer ?? '—'} · 授权 ${review.authority ?? '—'} · 证据 ${
        review.evidence ?? '—'
      }`,
      timestamp: review.completedAt ? formatInstant(review.completedAt) : undefined,
      variant: 'success',
    },
  ];
}

function queueStatusOf(entry: AcceptanceReviewQueueEntry) {
  return (
    <span className="shrink-0 text-[11px] text-idpxyz-textMuted">
      {entry.reviewCompleted ? '已录完成待续办' : '等待复核'}
    </span>
  );
}

/**
 * 接受前人工复核。队列语义取 parcel-shipment CONTEXT.md 的「等待人工复核」态：
 * 只有适用规则显式要求人工业务判断时，接受判断任务才进入该态，续办方是授权复核角色。
 *
 * 本页接 GET /acceptance-review-queue 的列表与单份两支，决定按钮打两个命令端点
 * （票 admin-skeleton-closure-batch/09）。命令面今天必答 403——那是 ADR-0055 的设计，
 * 页面如实呈现，不用开发用采信身份把它绕过去。
 */
export function AcceptanceReviewPage() {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [queueAnswer, setQueueAnswer] =
    useState<ApiResult<AcceptanceReviewQueueListResponseBody> | null>(null);
  const [caseLoaded, setCaseLoaded] = useState<{
    id: string;
    answer: ApiResult<AcceptanceReviewCaseResponseBody>;
  } | null>(null);
  const [commandPending, setCommandPending] = useState(false);
  const [commandNote, setCommandNote] = useState<CommandNote | null>(null);
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

  // 命令成立才重读：403 之后队列没有任何变化，重读只会白跑一趟。
  const settle = (note: CommandNote, reload: boolean) => {
    setCommandPending(false);
    setCommandNote(note);
    if (reload) retry();
  };

  // 两个命令各自 await 自己那一支：两支的响应体是不同的封闭形状，合成一个三元表达式
  // 会把它们并成联合类型，于是谁的 outcome 词表都对不上。
  const decide = (command: ReviewCommandId, shipmentRequestId: string, reason: string) => {
    setCommandPending(true);
    setCommandNote(null);
    if (command === 'complete') {
      void completeManualReview({ shipmentRequestId, reason }).then((answer) => {
        settle(commandNoteOf('记录复核完成', answer), answer.kind === 'outcome');
      });
      return;
    }
    void rejectShipmentRequest({ shipmentRequestId, reason }).then((answer) => {
      settle(commandNoteOf('主动拒绝委托', answer), answer.kind === 'outcome');
    });
  };

  // 「主动拒绝委托」按下之后再拦一道确认；「记录复核完成」只落留痕、决定由下一轮判断形成，不拦。
  // 模板交出理由的同时已清空输入框，所以取消确认时要说一声理由需重填，不让人以为它还在。
  const onDecide = ({ itemId, decision, reason }: { itemId: string; decision: ReviewCommandId; reason: string }) => {
    if (decision === 'reject') {
      const entry = entries.find((candidate) => candidate.shipmentRequestId === itemId);
      if (!entry) return;
      setCommandNote(null);
      setPendingRejection({ entry, reason });
      return;
    }
    decide(decision, itemId, reason);
  };
  const confirmRejection = () => {
    if (!pendingRejection) return;
    setPendingRejection(null);
    decide('reject', pendingRejection.entry.shipmentRequestId, pendingRejection.reason);
  };
  const cancelRejection = () => {
    setPendingRejection(null);
    setCommandNote({ tone: 'problem', text: '「主动拒绝委托」未发出：确认时取消。委托仍停在等待人工复核；复核理由已清空，要拒绝请重填。' });
  };

  return (
    <>
      <ReviewFlowTemplate<ReviewCommandId>
        title={info.title}
        description={`${info.owner}——停在「等待人工复核」的委托；复核完成只落留痕，决定由下一轮判断形成`}
        queueTitle="等待人工复核"
        queue={entries.map((entry) => ({
          id: entry.shipmentRequestId,
          title: entry.shipmentRequestId,
          subtitle: `${entry.customerAccountId} · ${entry.declaredParcelCount} 件`,
          status: queueStatusOf(entry),
          meta: formatInstant(entry.submittedAt),
        }))}
        selectedId={selectedId}
        onSelect={setSelectedId}
        detailTitle="复核详情"
        detailFields={caseBody ? caseDetailFields(caseBody) : undefined}
        detailExtra={
          <div className="flex flex-col gap-3">
            {/* 单份读不回时不留空白：详情是复核的依据，缺了要说清缺在哪一步。 */}
            {selectedId && !caseBody && (
              <p className="text-[12px] text-idpxyz-textMuted">
                {caseAnswer ? `复核详情未取回：${caseFailureText(caseAnswer)}` : '正在取回复核详情…'}
              </p>
            )}
            {caseBody && recordedJudgmentsBlock(caseBody.recordedJudgments)}
            {commandNote && (
              <p
                className={`text-[12px] ${
                  commandNote.tone === 'outcome' ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'
                }`}
              >
                {commandNote.text}
              </p>
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
          emptyTitle: '当前作用域没有等待人工复核的委托',
          emptyDescription:
            '读取入口已配置，队列为空——只有适用规则显式要求人工业务判断时委托才进入该队列，空队列是答案不是缺陷。',
        })}
      />
      {/* 「主动拒绝委托」当场形成决定、不可逆，按下之后再拦一道；确认即发命令，进行中由模板禁用按钮、结果由上面的 commandNote 呈现。 */}
      <ConfirmDialog
        open={pendingRejection !== null}
        tone="danger"
        title="主动拒绝委托"
        message={pendingRejection ? rejectionConfirmText(pendingRejection) : ''}
        confirmLabel="拒绝委托"
        cancelLabel="不拒绝"
        onConfirm={confirmRejection}
        onCancel={cancelRejection}
      />
    </>
  );
}
