import { useState } from 'react';
import { ConfirmDialog } from '@idpxyz/ui-primitives';
import { pendingText } from '../../components/action-feedback';
import {
  withdrawShipmentRequest,
  type ApiResult,
  type WithdrawalDraft,
  type WithdrawalResponseBody,
} from './api';
import {
  decisionKindLabels,
  requestStateLabels,
  withCode,
  withdrawalOutcomeViews,
  type OutcomeView,
} from './presentation';
import { DetailRow, Field, inputClass, NoticeCard, Section } from './controls';
import { ResultPanel } from './ResultPanel';

// UC-PS-005 决定前撤回入口。撤回只终止整份尚未决定的委托:已接受后的包裹取消
// 走 UC-PS-006,不在本页;部分成员的删改走新提交版本,也不在本页——放一个
// 「按包裹撤回」的口子就是替客户绕过整份判断。
//
// 撤回请求自己的来源信封与原提交分开(合用会被判成原提交的重放),两者都由接入
// 适配器从认证结果铸造;页面只声明客户可声明的部分:指名哪份委托、谁在请求、
// 依据什么原因。

interface WithdrawalFormState {
  originalRequestKey: string;
  shipmentRequestId: string;
  submissionVersionId: string;
  requesterReference: string;
  reasonReference: string;
}

const emptyForm: WithdrawalFormState = {
  originalRequestKey: '',
  shipmentRequestId: '',
  submissionVersionId: '',
  requesterReference: '',
  reasonReference: '',
};

// 必填集取 UC-PS-005 撤回请求的最低业务语义:目标委托、来源请求身份、请求方与
// 原因缺一即构造不出撤回请求。授权判定属服务端,页面不判。
function validate(form: WithdrawalFormState): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!form.originalRequestKey.trim()) errors.originalRequestKey = '必填';
  if (!form.shipmentRequestId.trim()) errors.shipmentRequestId = '必填';
  if (!form.requesterReference.trim()) errors.requesterReference = '必填';
  if (!form.reasonReference.trim()) errors.reasonReference = '必填';
  return errors;
}

function buildDraft(form: WithdrawalFormState): WithdrawalDraft {
  const version = form.submissionVersionId.trim();
  return {
    originalRequestKey: form.originalRequestKey.trim(),
    shipmentRequestId: form.shipmentRequestId.trim(),
    submissionVersionId: version === '' ? undefined : version,
    requesterReference: form.requesterReference.trim(),
    reasonReference: form.reasonReference.trim(),
  };
}

// 确认弹层的正文（票 admin-web-workspace-form/05 第 3 条，蓝图 21.2）：写明撤的是哪份委托、影响什么、以谁的名义,
// 不写「确定吗」。边界句取 UC-PS-005 的口径：撤回终止整份尚未决定的委托,不删除原始提交与判断历史。
function withdrawalConfirmText(form: WithdrawalFormState): string {
  const draft = buildDraft(form);
  const version = draft.submissionVersionId ? `,提交版本 ${draft.submissionVersionId}` : '';
  return (
    `将撤回委托 ${draft.shipmentRequestId}(来源请求 ${draft.originalRequestKey}${version})整份:` +
    `它不再等待接受或拒绝决定;原始提交与判断历史不删除,撤回本身记录在案。` +
    `以请求方 ${draft.requesterReference} 的名义,原因 ${draft.reasonReference}。`
  );
}

export function WithdrawShipmentRequestPage() {
  const [form, setForm] = useState<WithdrawalFormState>(emptyForm);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [result, setResult] = useState<ApiResult<WithdrawalResponseBody> | null>(null);

  const setField = (field: keyof WithdrawalFormState) => (value: string) =>
    setForm((current) => ({ ...current, [field]: value }));

  // 先校验再开确认:确认层问的是「要不要撤」,不是「填全了没」——两问挤在一层,没填全会被读成撤回被拒。
  function requestWithdraw() {
    const found = validate(form);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      setResult(null);
      return;
    }
    setConfirming(true);
  }

  async function handleWithdraw() {
    setConfirming(false);
    setPending(true);
    try {
      setResult(await withdrawShipmentRequest(buildDraft(form)));
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-[720px] mx-auto px-6 py-6 space-y-4">
        <header>
          <h1 className="text-[18px] font-bold text-idpxyz-textBright">
            撤回已提交的服务请求
          </h1>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-1">
            撤回只作用于整份尚未形成接受或拒绝决定的委托,不删除原始提交与判断历史,
            也不是运营企业拒绝(UC-PS-005)。委托已接受后的取消请求走接受后处置入口。
          </p>
        </header>

        <Section title="目标委托">
          <div className="grid grid-cols-2 gap-3">
            <Field
              label="原提交的来源请求标识"
              required
              error={errors.originalRequestKey}
              hint="产生该委托的那次提交所用的请求标识,用于定位目标委托。"
            >
              <input
                className={inputClass}
                value={form.originalRequestKey}
                onChange={(event) => setField('originalRequestKey')(event.target.value)}
              />
            </Field>
            <Field
              label="委托标识"
              required
              error={errors.shipmentRequestId}
              hint="提交成立时返回的 shipmentRequestId。"
            >
              <input
                className={inputClass}
                value={form.shipmentRequestId}
                onChange={(event) => setField('shipmentRequestId')(event.target.value)}
              />
            </Field>
            <Field
              label="当前提交版本标识"
              hint="可留空;留空时按服务端当前提交版本裁决。"
            >
              <input
                className={inputClass}
                value={form.submissionVersionId}
                onChange={(event) => setField('submissionVersionId')(event.target.value)}
              />
            </Field>
          </div>
        </Section>

        <Section title="撤回请求">
          <div className="grid grid-cols-2 gap-3">
            <Field
              label="请求方引用"
              required
              error={errors.requesterReference}
              hint="撤回授权的真实角色与范围待登记,此处只记录引用,授权由服务端判定。"
            >
              <input
                className={inputClass}
                value={form.requesterReference}
                onChange={(event) => setField('requesterReference')(event.target.value)}
              />
            </Field>
            <Field
              label="撤回原因引用"
              required
              error={errors.reasonReference}
              hint="原因目录属待登记参数,此处按引用填写,不提供预设选项。"
            >
              <input
                className={inputClass}
                value={form.reasonReference}
                onChange={(event) => setField('reasonReference')(event.target.value)}
              />
            </Field>
          </div>
        </Section>

        <div className="flex items-center gap-3">
          <button
            type="button"
            disabled={pending}
            onClick={requestWithdraw}
            className="rounded border border-idpxyz-accent px-4 py-2 text-[13px] font-bold text-idpxyz-accent hover:bg-idpxyz-hover disabled:opacity-50"
          >
            {pending ? pendingText('撤回') : '撤回整份委托'}
          </button>
          {Object.keys(errors).length > 0 ? (
            <span className="text-[12px] text-idpxyz-textBright">
              尚有输入不完整,已在字段旁标出。
            </span>
          ) : null}
        </div>

        <ResultPanel
          result={result}
          pending={pending}
          renderOutcome={(body, status) => (
            <WithdrawalOutcomeCard body={body} status={status} />
          )}
        />

        {/* 撤回是高风险动作(终止整份委托),按下之后再拦一道;确认即发请求,进行中与结果仍由上面的按钮与 ResultPanel 呈现。 */}
        <ConfirmDialog
          open={confirming}
          tone="danger"
          title="撤回整份委托"
          message={withdrawalConfirmText(form)}
          confirmLabel="撤回整份委托"
          cancelLabel="不撤回"
          onConfirm={handleWithdraw}
          onCancel={() => setConfirming(false)}
        />
      </div>
    </div>
  );
}

const unknownOutcome: OutcomeView = {
  label: '',
  note: '本页尚未收录该结果词,请以原词核对服务端记录与用例文档。',
};

function WithdrawalOutcomeCard({
  body,
  status,
}: {
  body: WithdrawalResponseBody;
  status: number;
}) {
  const view = withdrawalOutcomeViews[body.outcome] ?? unknownOutcome;
  return (
    <NoticeCard
      title={withCode(view.label, body.outcome)}
      titleClass={view.affirmative ? 'text-idpxyz-accent' : 'text-idpxyz-textBright'}
      meta={`HTTP ${status}${status === 201 ? ' · 撤回新建成立' : ''}`}
    >
      <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">{view.note}</p>

      <dl className="mt-3 space-y-1">
        {body.requestState ? (
          <DetailRow
            label="委托状态"
            value={withCode(requestStateLabels[body.requestState], body.requestState)}
          />
        ) : null}
        {body.withdrawalId ? (
          <DetailRow label="撤回标识" value={body.withdrawalId} mono />
        ) : null}
        {body.decisionKind ? (
          <DetailRow
            label="既有决定"
            value={withCode(decisionKindLabels[body.decisionKind], body.decisionKind)}
          />
        ) : null}
        {body.pendingReason ? (
          <DetailRow label="未决原因" value={body.pendingReason} mono />
        ) : null}
        {body.continuationReference ? (
          <DetailRow label="续办引用" value={body.continuationReference} mono />
        ) : null}
        {body.compensationReference ? (
          <DetailRow label="补偿续办引用" value={body.compensationReference} mono />
        ) : null}
      </dl>

      {body.compensationReference ? (
        <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-2">
          补偿续办与判断续办是两条路:此引用用于随附释放等补偿动作的续办,不用于重新判断。
        </p>
      ) : null}
    </NoticeCard>
  );
}
