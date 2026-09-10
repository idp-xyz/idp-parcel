import { useEffect, useState } from 'react';
import { Undo2, XCircle } from 'lucide-react';
import { ReviewFlowTemplate, type DetailField, type ReviewDecisionOption } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { formatInstant } from '../catalogue-view';
import {
  disposeShipmentRequest,
  findAcceptanceReviewCase,
  listAuthorizedDispositionQueue,
  type AcceptanceReviewCaseResponseBody,
  type ApiResult,
  type AuthorizedDispositionChoice,
  type AuthorizedDispositionQueueEntry,
  type AuthorizedDispositionQueueListResponseBody,
} from './api';
import {
  dispositionChoiceOptions,
  dispositionCommandNoteOf,
  dispositionQueueRowOf,
  dispositionQueueViewState,
  restrictedItemText,
  type DispositionCommandNote,
} from './authorized-disposition';
import { recordedJudgmentsBlock } from './recorded-judgments';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['authorized-disposition'];

/**
 * 决定集来自纯函数层的两去向，这里只配图标。两个图标都不是「对勾」：授权处置是对受限委托决定去向
 * （ADR-0132 决定一），不是对受限控制的表决——对勾会让处置角色以为自己在给控制放行。
 */
const dispositionDecisions: ReviewDecisionOption<AuthorizedDispositionChoice>[] = dispositionChoiceOptions.map(
  (option) => ({
    id: option.id,
    label: option.label,
    variant: option.variant,
    icon: option.id === 'REJECT' ? <XCircle size={13} /> : <Undo2 size={13} />,
  }),
);

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

function caseDetailFields(body: AcceptanceReviewCaseResponseBody, entry: AuthorizedDispositionQueueEntry): DetailField[] {
  const request = body.request;
  const fields: DetailField[] = [
    { label: '委托标识', value: <span className="font-mono">{request.shipmentRequestId}</span> },
    { label: '客户账户', value: <span className="font-mono">{request.customerAccountId}</span> },
    {
      label: '来源',
      value: <span className="font-mono">{`${request.source} / ${request.sourceRequestKey}`}</span>,
    },
    { label: '委托状态', value: <span className="font-mono">{request.state}</span> },
    { label: '提交版本', value: <span className="font-mono">{request.submissionVersionId}</span> },
    { label: '声明包裹', value: `${request.declaredParcelCount} 件` },
    { label: '提交时刻', value: formatInstant(request.submittedAt) },
    { label: '判断任务', value: request.acceptanceTask.state },
  ];
  if (entry.controlResultId) {
    fields.push({ label: '控制结果', value: <span className="font-mono">{entry.controlResultId}</span> });
  }
  // 最近一次没能推进的处理记录：有过才在场，没有就不占一格。
  if (entry.lastAttemptReason) {
    fields.push({
      label: '最近未推进',
      value: `${entry.lastAttemptReason}${
        entry.lastAttemptContinuation ? ` · 续办 ${entry.lastAttemptContinuation}` : ''
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

/**
 * 队列行上的受限项，逐条原词直显：处置角色选去向要看的正是这几行——哪一项受限、为什么、正文登记的
 * 失败处置是什么、责任在谁（ADR-0132 决定四：责任引用答「谁承担失败或补偿责任」，不答「谁有权处置」）。
 * 空数组不写「无受限项」——停在等处置的委托不该没有受限项，如实标坏数据征兆。
 */
function restrictedItemsBlock(entry: AuthorizedDispositionQueueEntry) {
  return (
    <div className="flex flex-col gap-1 text-[12px]">
      <span className="text-idpxyz-textMuted">受限控制项（本版本采用的正文登记）</span>
      {entry.restrictedItems.length === 0 ? (
        <p className="text-idpxyz-textMuted">
          受限项为空数组——停在等待授权处置的委托不该没有受限项，这是坏数据的可观察征兆，不是「没有受限项」。
        </p>
      ) : (
        <ul className="flex flex-col gap-0.5">
          {entry.restrictedItems.map((item) => (
            <li key={`${item.kind}:${item.order}`} className="font-mono">
              {restrictedItemText(item)}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/**
 * 授权处置。队列语义取 parcel-shipment CONTEXT.md 的「等待授权处置」态（ADR-0132 决定二）：
 * 接受前财务控制按共同通过条件不成立、且全部受限项在策略正文里登记为进入授权处置时，接受判断任务
 * 停在该态，续办方是授权处置角色，完成动作是选去向。
 *
 * 本页读 GET /authorized-disposition-queue 的列表；单份详情复用复核队列的 `?shipmentRequestId=` 分支
 * （读面只有列表，那里透出的判断三组正是处置角色要审的）；两个去向打 POST
 * /shipment-requests/authorized-dispositions。决定集只有`拒绝`与`交客户补充`——「放行」不在集内，
 * 硬句「人工处理不得绕过硬规则或把缺少的权威结果改成通过」在这里是结构性的：去向类型只有两值。
 * 命令面今天必答 403（ADR-0085 / ADR-0055 未配置档），页面如实呈现，不用开发用采信身份绕过；
 * 谁有权处置由编排去问 party-commercial，前端不自判。
 */
export function AuthorizedDispositionPage() {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [queueAnswer, setQueueAnswer] =
    useState<ApiResult<AuthorizedDispositionQueueListResponseBody> | null>(null);
  const [caseLoaded, setCaseLoaded] = useState<{
    id: string;
    answer: ApiResult<AcceptanceReviewCaseResponseBody>;
  } | null>(null);
  const [commandPending, setCommandPending] = useState(false);
  const [commandNote, setCommandNote] = useState<DispositionCommandNote | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listAuthorizedDispositionQueue().then((answer) => {
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
  const selectedEntry = entries.find((entry) => entry.shipmentRequestId === selectedId) ?? null;
  const caseAnswer = caseLoaded?.id === selectedId ? caseLoaded.answer : null;
  const caseBody = caseAnswer?.kind === 'outcome' ? caseAnswer.body : null;
  const retry = () => setReloadKey((value) => value + 1);

  // 命令成立才重读：403 之后队列没有任何变化，重读只会白跑一趟。
  const decide = (choice: AuthorizedDispositionChoice, entry: AuthorizedDispositionQueueEntry, reason: string) => {
    setCommandPending(true);
    setCommandNote(null);
    const label = dispositionChoiceOptions.find((option) => option.id === choice)?.label ?? choice;
    // 版本取队列行上看到的那一份：处置签在版本的判断任务上，换代后由服务端答`版本已换代`，页面不猜当前版。
    void disposeShipmentRequest({
      shipmentRequestId: entry.shipmentRequestId,
      submissionVersionId: entry.submissionVersionId,
      choice,
      reason,
    }).then((answer) => {
      setCommandPending(false);
      setCommandNote(dispositionCommandNoteOf(label, answer));
      if (answer.kind === 'outcome') retry();
    });
  };

  return (
    <ReviewFlowTemplate<AuthorizedDispositionChoice>
      title={info.title}
      description={`${info.owner}——停在「等待授权处置」的委托；处置只决定去向（拒绝 / 交客户补充），从不形成接受`}
      queueTitle="等待授权处置"
      queue={entries.map((entry) => {
        const row = dispositionQueueRowOf(entry);
        return {
          id: row.id,
          title: row.title,
          subtitle: row.subtitle,
          status: (
            <span
              className={`shrink-0 text-[11px] ${row.badData ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'}`}
            >
              {row.statusText}
            </span>
          ),
          meta: row.meta,
        };
      })}
      selectedId={selectedId}
      onSelect={setSelectedId}
      detailTitle="处置详情"
      detailFields={caseBody && selectedEntry ? caseDetailFields(caseBody, selectedEntry) : undefined}
      detailExtra={
        <div className="flex flex-col gap-3">
          {selectedEntry && restrictedItemsBlock(selectedEntry)}
          {/* 单份读不回时不留空白：已记录的判断是处置的依据，缺了要说清缺在哪一步。 */}
          {selectedId && !caseBody && (
            <p className="text-[12px] text-idpxyz-textMuted">
              {caseAnswer ? `已记录判断未取回：${caseFailureText(caseAnswer)}` : '正在取回已记录的判断…'}
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
      decisions={dispositionDecisions}
      decisionPending={commandPending}
      onDecide={({ itemId, decision, reason }) => {
        const entry = entries.find((candidate) => candidate.shipmentRequestId === itemId);
        if (entry) decide(decision, entry, reason);
      }}
      viewState={dispositionQueueViewState(queueAnswer, retry, info)}
    />
  );
}
