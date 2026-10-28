import { useEffect, useState } from 'react';
import { CheckCheck, Landmark } from 'lucide-react';
import { ConfirmDialog } from '@idpxyz/ui-primitives';
import { ReviewFlowTemplate, type DetailField, type ReviewDecisionOption } from '../../templates';
import { ReloginButton } from '../../auth/ReloginButton';
import { TechnicalDetails } from '../../components/TechnicalDetails';
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
  refusalTechnicalRows,
  type RefusalHandlingNote,
} from './control-release-refusals';
import { ShipmentNo } from './refs';
import { shipmentNoOf } from './ref-no';

// 页面标题取导航登记的那一份;主责上下文(parcel-shipment)只在代码里出现,屏上不说系统的话。
const info = moduleInfoById['control-release-refusals'];

// 决定集来自纯函数层的两种动作,这里只配图标。「依据」就是模板的必填理由:处理人核对了什么、凭什么认定这一笔已收口。
const actionDecisions: ReviewDecisionOption<ControlReleaseRefusalAction>[] = refusalActionOptions.map((option) => ({
  id: option.id,
  label: option.label,
  variant: option.variant,
  icon: option.id === 'RELEASE_AT_SETTLEMENT' ? <Landmark size={13} /> : <CheckCheck size={13} />,
}));

function detailFieldsOf(row: ControlReleaseRefusalRow): DetailField[] {
  return [
    { label: '运单号', value: <ShipmentNo shipmentRequestId={row.shipmentRequestId} /> },
    { label: '客户', value: row.customerAccountId },
    { label: '占用在哪', value: refusalScopeText(row) },
    { label: '登记时间', value: formatInstant(row.recordedAt) },
  ];
}

/** 待确认的「标记已处理」:模板在 onDecide 时已把依据交出并清空输入框,这里把它和目标行一起攥住到确认或取消。 */
interface PendingMark {
  row: ControlReleaseRefusalRow;
  basis: string;
}

// 确认弹层的正文:写明标的是哪一笔、标记意味着什么、依据是什么,不写「确定吗」。标记只留痕、不动结算账本,
// 却会让接受链此后不再为这一笔停下——所以说清它不会替人把钱放掉。
function markConfirmText({ row, basis }: PendingMark): string {
  return (
    `将把运单 ${shipmentNoOf(row.shipmentRequestId)}(客户 ${row.customerAccountId})被结算侧拒绝释放的那笔占用标记为已处理。` +
    '标记只记一笔,不会动账、不会替你放掉这笔钱;之后系统不再为这一笔停下,会接着处理这票运单。' +
    `记录不能撤销。依据:${basis}。`
  );
}

/**
 * 资金释放待处理(释放未受理册,ADR-0293)。册上是撤回、拒绝、授权处置、自动拒绝与停进等待受控补充各路遇到结算确定不受理的
 * 释放时记下的占用(ADR-0282 决定二):业务事实已成立,这一笔钱却还占着客户的余额或额度,接受链那一路的新版本也可能因它判不了。
 *
 * 读 GET /control-release-refusals(按租户列未处理的行并计数);两种动作打 POST /control-release-refusal-handlings:
 * 「核对后释放」由服务端按原关联再请求一次释放,提供方答已收口才算处理;「标记已处理」只留痕。两者都同笔让接受链
 * 重驱那份委托的当前版本。没有告警阈值——本产品没有告警引擎,页头只给计数。谁有权处理由「运营决定」里「释放拒收处理」
 * 这一种授予把守(与授权处置的授予分开),前端不自判。
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

  // 处理有了答复就重读:成立的那一笔要从清单消失,已被处理的也一样;释放没收口 / 再被拒收时清单不变,重读只是确认没变。
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
    setNote({ tone: 'problem', text: '没有标记。这一笔仍待处理;依据已清空,要标记请重填。', technical: [] });
  };

  return (
    <>
      <ReviewFlowTemplate<ControlReleaseRefusalAction>
        title={info.title}
        description={`运单的业务结果已经定了,但占用客户余额或额度的那笔钱结算侧没放掉。在这里核对后释放,或在结算侧手工放掉后标记已处理。${body ? refusalSummaryText(body) : ''}`}
        queueTitle="待处理的占用"
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
        detailTitle="处理这笔占用"
        detailFields={selected ? detailFieldsOf(selected) : undefined}
        detailExtra={
          <div className="flex flex-col gap-3">
            {selected && (
              <p className="text-[12px] text-idpxyz-textMuted">
                在下方写明核对了什么。「{refusalActionLabel('RELEASE_AT_SETTLEMENT')}」会再向结算侧申请放掉这笔钱,结算侧确认放掉才算处理完;
                「{refusalActionLabel('MARK_HANDLED')}」只记一笔(谁、凭什么、什么时候),不动账,用于已经在结算侧手工放掉的情况。处理完后系统会接着处理这票运单。
              </p>
            )}
            {note && (
              <div className={`text-[12px] ${note.tone === 'outcome' ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'}`}>
                {note.text}
                {note.tone === 'refused' && note.kind === 'loginRequired' ? <ReloginButton size="inline" /> : null}
                <TechnicalDetails rows={note.technical} />
              </div>
            )}
            {selected && <TechnicalDetails rows={refusalTechnicalRows(selected)} />}
          </div>
        }
        decisions={actionDecisions}
        decisionPending={pending}
        onDecide={onDecide}
        viewState={refusalQueueViewState(answer, retry, info)}
      />
      {/* 标记已处理只留痕却会让接受链此后不再为这一笔停下,按下之后再拦一道;确认即发命令,进行中由模板禁用按钮、结果由 `note` 状态呈现。 */}
      <ConfirmDialog
        renderFooter={confirmDialogFooter}
        open={pendingMark !== null}
        tone="danger"
        title="标记这笔占用已处理"
        message={pendingMark ? markConfirmText(pendingMark) : ''}
        confirmLabel="标记已处理"
        cancelLabel="先不标记"
        onConfirm={confirmMark}
        onCancel={cancelMark}
      />
    </>
  );
}
