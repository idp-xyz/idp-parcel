import { useEffect, useState } from 'react';
import { Undo2, XCircle } from 'lucide-react';
import { ConfirmDialog } from '@idpxyz/ui-primitives';
import { ReviewFlowTemplate, type ReviewDecisionOption } from '../../templates';
import { ReloginButton } from '../../auth/ReloginButton';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { moduleInfoById } from '../../navigation';
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
  restrictedItemTechnicalRows,
  restrictedItemText,
  type DispositionCommandNote,
} from './authorized-disposition';
import { recordedJudgmentsBlock } from './recorded-judgments';
import { caseBusinessFields, caseFailureText, caseTechnicalRows } from './review-case';
import { shipmentNoOf } from './ref-no';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';

// 页面标题取导航登记的那一份;主责上下文(parcel-shipment)只在代码里出现,屏上不说系统的话。
const info = moduleInfoById['authorized-disposition'];

/**
 * 决定集来自纯函数层的两去向,这里只配图标。两个图标都不是「对勾」:授权处置是对受限委托决定去向
 * (ADR-0132 决定一),不是对受限控制的表决——对勾会让处置角色以为自己在给控制放行。
 */
const dispositionDecisions: ReviewDecisionOption<AuthorizedDispositionChoice>[] = dispositionChoiceOptions.map((option) => ({
  id: option.id,
  label: option.label,
  variant: option.variant,
  icon: option.id === 'REJECT' ? <XCircle size={13} /> : <Undo2 size={13} />,
}));

/** 待确认的「拒绝」:模板在 onDecide 时已把理由交出并清空输入框,这里把它和目标行一起攥住到确认或取消。 */
interface PendingRejection {
  entry: AuthorizedDispositionQueueEntry;
  reason: string;
}

// 确认弹层的正文(票 admin-web-workspace-form/05 第 3 条,蓝图 21.2):写明拒的是哪票运单、影响谁、按什么理由,不写「确定吗」。
// 「拒绝」是对受限委托形成的去向决定(ADR-0132 决定一),不是对受限控制的表决——正文照这条说,不说「驳回」。
function rejectionConfirmText({ entry, reason }: PendingRejection): string {
  return (
    `将拒绝运单 ${shipmentNoOf(entry.shipmentRequestId)}(客户 ${entry.customerAccountId},${entry.declaredParcelCount} 件)。` +
    `拒绝立即生效,之后不会再回到等待处置,也不会变成受理;理由:${reason}。`
  );
}

/**
 * 队列行上的受限项,逐条说运营的话:处置角色选去向要看的正是这几行——哪项检查没过、为什么、策略说不过时怎么办、
 * 责任在谁(ADR-0132 决定四:责任引用答「谁承担失败或补偿责任」,不答「谁有权处置」)。
 * 空数组不写「没有受限项」——停在等处置的运单不该没有受限项,如实标数据异常。
 */
function restrictedItemsBlock(entry: AuthorizedDispositionQueueEntry) {
  return (
    <div className="flex flex-col gap-1 text-[12px]">
      <span className="text-idpxyz-textMuted">没通过的检查</span>
      {entry.restrictedItems.length === 0 ? (
        <p className="text-idpxyz-textMuted">这票运单停在等待处置,却没有任何没通过的检查——数据异常,请联系管理员核对。</p>
      ) : (
        <ul className="flex flex-col gap-0.5">
          {entry.restrictedItems.map((item) => (
            <li key={`${item.kind}:${item.order}`}>{restrictedItemText(item)}</li>
          ))}
        </ul>
      )}
    </div>
  );
}

function NoteLine({ note }: { note: DispositionCommandNote | null }) {
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
 * 授权处置。队列语义取 parcel-shipment CONTEXT.md 的「等待授权处置」态(ADR-0132 决定二):
 * 接受前财务控制按共同通过条件不成立、且全部受限项在策略正文里登记为进入授权处置时,接受判断任务
 * 停在该态,续办方是授权处置角色,完成动作是选去向。
 *
 * 本页读 GET /authorized-disposition-queue 的列表;单份详情复用复核队列的 `?shipmentRequestId=` 分支
 * (读面只有列表,那里透出的判断三组正是处置角色要审的);两个去向打 POST
 * /shipment-requests/authorized-dispositions。决定集只有`拒绝`与`交客户补充`——「放行」不在集内,
 * 硬句「人工处理不得绕过硬规则或把缺少的权威结果改成通过」在这里是结构性的:去向类型只有两值。
 * 谁有权处置由编排去问 party-commercial,前端不自判。
 */
export function AuthorizedDispositionPage() {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [queueAnswer, setQueueAnswer] = useState<ApiResult<AuthorizedDispositionQueueListResponseBody> | null>(null);
  const [caseLoaded, setCaseLoaded] = useState<{ id: string; answer: ApiResult<AcceptanceReviewCaseResponseBody> } | null>(null);
  const [commandPending, setCommandPending] = useState(false);
  const [commandNote, setCommandNote] = useState<DispositionCommandNote | null>(null);
  const [pendingRejection, setPendingRejection] = useState<PendingRejection | null>(null);

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

  // 命令成立才重读:403 之后队列没有任何变化,重读只会白跑一趟。
  const decide = (choice: AuthorizedDispositionChoice, entry: AuthorizedDispositionQueueEntry, reason: string) => {
    setCommandPending(true);
    setCommandNote(null);
    const label = dispositionChoiceOptions.find((option) => option.id === choice)?.label ?? choice;
    // 版本取队列行上看到的那一份:处置签在版本的判断任务上,换代后由服务端答`版本已换代`,页面不猜当前版。
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

  // 「拒绝」按下之后再拦一道确认;「交客户补充」把运单交回客户续办,不是不可逆去向,不拦。
  // 模板交出理由的同时已清空输入框,所以取消确认时要说一声理由需重填,不让人以为它还在。
  const onDecide = ({ itemId, decision, reason }: { itemId: string; decision: AuthorizedDispositionChoice; reason: string }) => {
    const entry = entries.find((candidate) => candidate.shipmentRequestId === itemId);
    if (!entry) return;
    if (decision === 'REJECT') {
      setCommandNote(null);
      setPendingRejection({ entry, reason });
      return;
    }
    decide(decision, entry, reason);
  };
  const confirmRejection = () => {
    if (!pendingRejection) return;
    setPendingRejection(null);
    decide('REJECT', pendingRejection.entry, pendingRejection.reason);
  };
  const cancelRejection = () => {
    setPendingRejection(null);
    setCommandNote({ tone: 'problem', text: '没有拒绝。运单仍在等待处置;理由已清空,要拒绝请重填。', technical: [] });
  };

  return (
    <>
      <ReviewFlowTemplate<AuthorizedDispositionChoice>
        title={info.title}
        description="受理前的付款检查没通过、要由有权的人决定去向的运单停在这里。只能选拒绝或交客户补充,这里不能代替检查放行。"
        queueTitle="等待处置的运单"
        queue={entries.map((entry) => {
          const row = dispositionQueueRowOf(entry);
          return {
            id: row.id,
            title: row.title,
            subtitle: row.subtitle,
            status: <span className={`shrink-0 text-[11px] ${row.badData ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'}`}>{row.statusText}</span>,
            meta: row.meta,
          };
        })}
        selectedId={selectedId}
        onSelect={setSelectedId}
        detailTitle="决定这票运单的去向"
        detailFields={caseBody ? caseBusinessFields(caseBody.request) : undefined}
        detailExtra={
          <div className="flex flex-col gap-3">
            {selectedEntry && restrictedItemsBlock(selectedEntry)}
            {/* 单份读不回时不留空白:已记录的判断是处置的依据,缺了要说清缺在哪一步。 */}
            {selectedId && !caseBody && (
              <p className="text-[12px] text-idpxyz-textMuted">{caseAnswer ? caseFailureText(caseAnswer) : '正在读取详情…'}</p>
            )}
            {caseBody && recordedJudgmentsBlock(caseBody.recordedJudgments)}
            <NoteLine note={commandNote} />
            {caseBody && selectedEntry && (
              <TechnicalDetails
                rows={caseTechnicalRows(caseBody.request, [
                  { label: '控制结果', value: selectedEntry.controlResultId ?? '' },
                  ...restrictedItemTechnicalRows(selectedEntry.restrictedItems),
                ])}
              />
            )}
          </div>
        }
        decisions={dispositionDecisions}
        decisionPending={commandPending}
        onDecide={onDecide}
        viewState={dispositionQueueViewState(queueAnswer, retry, info)}
      />
      {/* 「拒绝」是不可逆去向,按下之后再拦一道;确认即发命令,进行中由模板禁用按钮、结果由上面的 commandNote 呈现。 */}
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
