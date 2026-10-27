import { useEffect, useState } from 'react';
import { CheckCheck, Landmark } from 'lucide-react';
import { ConfirmDialog } from '@idpxyz/ui-primitives';
import { ReviewFlowTemplate, type DetailField, type ReviewDecisionOption } from '../../templates';
import { ReloginButton } from '../../auth/ReloginButton';
import { moduleInfoById } from '../../navigation';
import { formatInstant } from '../catalogue-view';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import {
  handleControlReleaseRefusal,
  listControlReleaseRefusals,
  type ApiResult,
  type ControlReleaseRefusalAction,
  type ControlReleaseRefusalListResponseBody,
  type ControlReleaseRefusalRow,
} from './api';
import {
  handlingDraftOf,
  refusalActionLabel,
  refusalActionOptions,
  refusalHandlingNoteOf,
  refusalKeyOf,
  refusalQueueRowOf,
  refusalQueueViewState,
  refusalScopeText,
  refusalSummaryText,
  type RefusalHandlingNote,
} from './control-release-refusals';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['control-release-refusals'];

// 决定集来自纯函数层的两种动作，这里只配图标。「依据」就是模板的必填理由：处理人核对了什么、凭什么认定这一笔已收口。
const actionDecisions: ReviewDecisionOption<ControlReleaseRefusalAction>[] = refusalActionOptions.map((option) => ({
  id: option.id,
  label: option.label,
  variant: option.variant,
  icon: option.id === 'RELEASE_AT_SETTLEMENT' ? <Landmark size={13} /> : <CheckCheck size={13} />,
}));

function detailFieldsOf(row: ControlReleaseRefusalRow): DetailField[] {
  return [
    { label: '委托标识', value: <span className="font-mono">{row.shipmentRequestId}</span> },
    { label: '客户账户', value: <span className="font-mono">{row.customerAccountId}</span> },
    { label: '被拒收的提交版本', value: <span className="font-mono">{row.submissionVersionId}</span> },
    { label: '控制关联', value: <span className="font-mono">{row.controlResultId}</span> },
    { label: '存下的作用域', value: refusalScopeText(row) },
    { label: '提供方给的原因', value: <span className="font-mono">{row.reason}</span> },
    { label: '登记时刻', value: formatInstant(row.recordedAt) },
  ];
}

/** 待确认的「标记已处理」：模板在 onDecide 时已把依据交出并清空输入框，这里把它和目标行一起攥住到确认或取消。 */
interface PendingMark {
  row: ControlReleaseRefusalRow;
  basis: string;
}

// 确认弹层的正文：写明标的是哪一笔、标记意味着什么、依据是什么，不写「确定吗」。标记只留痕、不动结算账本，
// 却会让接受链此后不再为这一笔停下——所以说清它不会替人把钱放掉。
function markConfirmText({ row, basis }: PendingMark): string {
  return (
    `将把委托 ${row.shipmentRequestId}（客户账户 ${row.customerAccountId}）被拒收的占用——版本 ${row.submissionVersionId}，控制关联 ${row.controlResultId}——标记为已处理。` +
    '标记只留痕，不会动结算账本、不会替你释放这笔资金；此后接受链遇到提供方对这一笔再次拒收时不再停下，并立即拿这份委托的当前版本再驱一拍。' +
    `留痕只追加，不能撤销。依据：${basis}。`
  );
}

/**
 * 释放拒收处理（ADR-0293）。册上是撤回、拒绝、授权处置、自动拒绝与停进等待受控补充各路遇到结算确定不受理的释放时记下的占用
 * （ADR-0282 决定二）：业务事实已成立，这一笔钱却还占着客户的余额或额度，接受链那一路的新版本也可能因它判不了。
 *
 * 读 GET /control-release-refusals（按租户列未处理的行并计数）；两种动作打 POST /control-release-refusal-handlings：
 * 「核对后在结算侧释放」由服务端按原关联再请求一次释放，提供方答已收口才算处理；「标记已处理」只留痕。两者都同笔让接受链
 * 重驱那份委托的当前版本。没有告警阈值——本产品没有告警引擎，页头只给计数。谁有权处理由「运营决定」里「释放拒收处理」
 * 这一种授予把守（与授权处置的授予分开），前端不自判。
 */
export function ControlReleaseRefusalsPage() {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<ControlReleaseRefusalListResponseBody> | null>(null);
  const [pending, setPending] = useState(false);
  const [note, setNote] = useState<RefusalHandlingNote | null>(null);
  const [pendingMark, setPendingMark] = useState<PendingMark | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listControlReleaseRefusals().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const refusals = body?.refusals ?? [];
  const selected = refusals.find((row) => refusalKeyOf(row) === selectedId) ?? null;
  const retry = () => setReloadKey((value) => value + 1);

  // 处理有了答复就重读：成立的那一笔要从清单消失，已被处理的也一样；释放没收口 / 再被拒收时清单不变，重读只是确认没变。
  const act = (action: ControlReleaseRefusalAction, row: ControlReleaseRefusalRow, basis: string) => {
    setPending(true);
    setNote(null);
    const label = refusalActionLabel(action);
    void handleControlReleaseRefusal(handlingDraftOf(row, action, basis)).then((result) => {
      setPending(false);
      setNote(refusalHandlingNoteOf(label, result));
      if (result.kind === 'outcome') {
        if (result.body.outcome === 'HANDLED' || result.body.outcome === 'ALREADY_HANDLED') setSelectedId(null);
        retry();
      }
    });
  };

  const onDecide = ({ itemId, decision, reason }: { itemId: string; decision: ControlReleaseRefusalAction; reason: string }) => {
    const row = refusals.find((candidate) => refusalKeyOf(candidate) === itemId);
    if (!row) return;
    if (decision === 'MARK_HANDLED') {
      setNote(null);
      setPendingMark({ row, basis: reason });
      return;
    }
    act(decision, row, reason);
  };
  const confirmMark = () => {
    if (!pendingMark) return;
    setPendingMark(null);
    act('MARK_HANDLED', pendingMark.row, pendingMark.basis);
  };
  const cancelMark = () => {
    setPendingMark(null);
    setNote({ tone: 'problem', text: '「标记已处理」未发出：确认时取消。这一笔仍未处理；依据已清空，要标记请重填。' });
  };

  return (
    <>
      <ReviewFlowTemplate<ControlReleaseRefusalAction>
        title={info.title}
        description={`${info.owner}——被结算确定不受理的释放：业务事实已成立，这一笔占用仍在。${body ? refusalSummaryText(body) : ''}`}
        queueTitle="未处理的释放拒收"
        queue={refusals.map((row) => {
          const line = refusalQueueRowOf(row);
          return {
            id: line.id,
            title: line.title,
            subtitle: line.subtitle,
            status: <span className="shrink-0 text-[11px] text-idpxyz-textMuted">{line.statusText}</span>,
            meta: line.meta,
          };
        })}
        selectedId={selectedId}
        onSelect={setSelectedId}
        detailTitle="拒收详情"
        detailFields={selected ? detailFieldsOf(selected) : undefined}
        detailExtra={
          <div className="flex flex-col gap-3">
            {selected && (
              <p className="text-[12px] text-idpxyz-textMuted">
                「依据」写核对了什么：{refusalActionLabel('RELEASE_AT_SETTLEMENT')}由服务端按原关联与存下的作用域再请求一次释放，提供方答已收口才算处理；
                {refusalActionLabel('MARK_HANDLED')}只留痕（谁、凭什么、何时），不动结算账本。两者都会让接受链重驱这份委托的当前版本。
              </p>
            )}
            {note && (
              <p className={`text-[12px] ${note.tone === 'outcome' ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'}`}>
                {note.text}
                {note.tone === 'refused' && note.kind === 'loginRequired' ? <ReloginButton size="inline" /> : null}
              </p>
            )}
          </div>
        }
        decisions={actionDecisions}
        decisionPending={pending}
        onDecide={onDecide}
        viewState={refusalQueueViewState(answer, retry, info)}
      />
      {/* 标记已处理只留痕却会让接受链此后不再为这一笔停下，按下之后再拦一道；确认即发命令，进行中由模板禁用按钮、结果由上面的 note 呈现。 */}
      <ConfirmDialog
        renderFooter={confirmDialogFooter}
        open={pendingMark !== null}
        tone="danger"
        title="标记释放拒收已处理"
        message={pendingMark ? markConfirmText(pendingMark) : ''}
        confirmLabel="标记已处理"
        cancelLabel="不标记"
        onConfirm={confirmMark}
        onCancel={cancelMark}
      />
    </>
  );
}
