import { useState } from 'react';
import { ConfirmDialog } from '@idpxyz/ui-primitives';
import { pendingText } from '../../components/action-feedback';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { StatusPill } from '../../components/StatusPill';
import { withdrawShipmentRequest, type ApiResult, type WithdrawalDraft, type WithdrawalResponseBody } from './api';
import { withdrawalOutcomeViews, type OutcomeView } from './presentation';
import { Field, inputClass, NoticeCard, Section } from './controls';
import { shipmentNoOf } from './ref-no';
import { ResultPanel } from './ResultPanel';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';

// 撤回运单(UC-PS-005 决定前撤回入口)。撤回只终止整份尚未决定的委托:已接受后的包裹取消
// 走 UC-PS-006,不在本页;部分成员的删改走新提交版本,也不在本页——放一个
// 「按包裹撤回」的口子就是替客户绕过整份判断。
//
// 撤回请求自己的来源信封与原提交分开(合用会被判成原提交的重放),两者都由接入
// 适配器从认证结果铸造;页面只声明客户可声明的部分:指名哪份委托、谁在请求、
// 依据什么原因。屏上「运单号」是委托标识,「客户单号」是原提交的来源请求键。

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
  if (!form.shipmentRequestId.trim()) errors.shipmentRequestId = '请填写运单号。';
  if (!form.originalRequestKey.trim()) errors.originalRequestKey = '请填写客户单号。';
  if (!form.requesterReference.trim()) errors.requesterReference = '请填写是谁提出撤回。';
  if (!form.reasonReference.trim()) errors.reasonReference = '请填写撤回原因。';
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

// 确认弹层的正文(票 admin-web-workspace-form/05 第 3 条,蓝图 21.2):写明撤的是哪票运单、影响什么、以谁的名义,
// 不写「确定吗」。边界句取 UC-PS-005 的口径:撤回终止整份尚未决定的委托,不删除原始提交与判断历史。
function withdrawalConfirmText(form: WithdrawalFormState): string {
  const draft = buildDraft(form);
  return (
    `将撤回运单 ${shipmentNoOf(draft.shipmentRequestId)}(客户单号 ${draft.originalRequestKey})整票:` +
    `它不再等受理;录过的内容与记录都保留,撤回本身也记在案。` +
    `申请人 ${draft.requesterReference},原因:${draft.reasonReference}。`
  );
}

export function WithdrawShipmentRequestPage() {
  const [form, setForm] = useState<WithdrawalFormState>(emptyForm);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [result, setResult] = useState<ApiResult<WithdrawalResponseBody> | null>(null);

  const setField = (field: keyof WithdrawalFormState) => (value: string) => setForm((current) => ({ ...current, [field]: value }));

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
          <h1 className="text-[18px] font-bold text-idpxyz-textBright">撤回运单</h1>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-1">
            客户在运单受理之前要停掉它,在这里整票撤回;录过的内容不会删。已经受理的运单要停,请走「取消运单」。
          </p>
        </header>

        <Section title="哪一票运单">
          <div className="grid grid-cols-2 gap-3">
            <Field label="运单号" required error={errors.shipmentRequestId} hint="提交时给客户的运单号。">
              <input className={`${inputClass} font-mono`} value={form.shipmentRequestId} onChange={(event) => setField('shipmentRequestId')(event.target.value)} />
            </Field>
            <Field label="客户单号" required error={errors.originalRequestKey} hint="客户下单时自己的单号;与运单号一起才能找到这票运单。">
              <input className={`${inputClass} font-mono`} value={form.originalRequestKey} onChange={(event) => setField('originalRequestKey')(event.target.value)} />
            </Field>
            <Field label="哪一版" hint="客户改过资料时才需要填版本;不填就按最新一版。">
              <input className={`${inputClass} font-mono`} value={form.submissionVersionId} onChange={(event) => setField('submissionVersionId')(event.target.value)} />
            </Field>
          </div>
        </Section>

        <Section title="谁要撤、为什么">
          <div className="grid grid-cols-2 gap-3">
            <Field label="谁提出撤回" required error={errors.requesterReference} hint="客户或联系人的名字 / 编号,记在撤回记录里。">
              <input className={inputClass} value={form.requesterReference} onChange={(event) => setField('requesterReference')(event.target.value)} />
            </Field>
            <Field label="撤回原因" required error={errors.reasonReference} hint="一句话说明,记在撤回记录里。">
              <input className={inputClass} value={form.reasonReference} onChange={(event) => setField('reasonReference')(event.target.value)} />
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
            {pending ? pendingText('撤回') : '撤回这票运单'}
          </button>
          {Object.keys(errors).length > 0 ? <span className="text-[12px] text-idpxyz-textBright">还有没填好的地方,已在字段旁标出。</span> : null}
        </div>

        <ResultPanel result={result} pending={pending} renderOutcome={(body, status) => <WithdrawalOutcomeCard body={body} status={status} />} />

        {/* 撤回是高风险动作(终止整份委托),按下之后再拦一道;确认即发请求,进行中与结果仍由上面的按钮与 ResultPanel 呈现。 */}
        <ConfirmDialog
          renderFooter={confirmDialogFooter}
          open={confirming}
          tone="danger"
          title="撤回这票运单"
          message={withdrawalConfirmText(form)}
          confirmLabel="确认撤回"
          cancelLabel="先不撤回"
          onConfirm={handleWithdraw}
          onCancel={() => setConfirming(false)}
        />
      </div>
    </div>
  );
}

const unknownOutcome: OutcomeView = {
  label: '结果未收录',
  note: '系统交回了本页还不认识的结果,请按技术详情里的结果码核对记录。',
};

function WithdrawalOutcomeCard({ body, status }: { body: WithdrawalResponseBody; status: number }) {
  const view = withdrawalOutcomeViews[body.outcome] ?? unknownOutcome;
  return (
    <NoticeCard title={view.label} titleClass={view.affirmative ? 'text-idpxyz-accent' : 'text-idpxyz-textBright'}>
      <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">{view.note}</p>

      <dl className="mt-3 space-y-1">
        {body.requestState ? (
          <div className="flex gap-2 text-[12px] leading-5">
            <dt className="text-idpxyz-textMuted shrink-0 w-[104px]">运单现在</dt>
            <dd>
              <StatusPill code={body.requestState} />
            </dd>
          </div>
        ) : null}
        {body.decisionKind ? (
          <div className="flex gap-2 text-[12px] leading-5">
            <dt className="text-idpxyz-textMuted shrink-0 w-[104px]">已有决定</dt>
            <dd>
              <StatusPill code={body.decisionKind} />
            </dd>
          </div>
        ) : null}
      </dl>

      {body.compensationReference ? (
        <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-2">占用的资金还没释放完,系统会继续处理,不用重新撤回。</p>
      ) : null}
      {body.controlReleaseNotAccepted ? (
        <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-2">撤回已成立;占用的资金结算侧没有受理释放,已转到「资金释放待处理」等人处理,再撤一次不会改变它。</p>
      ) : null}

      <TechnicalDetails
        rows={[
          { label: 'HTTP 状态', value: String(status) },
          { label: '结果码', value: body.outcome },
          { label: '委托状态码', value: body.requestState ?? '' },
          { label: '撤回标识', value: body.withdrawalId ?? '' },
          { label: '未决原因', value: body.pendingReason ?? '' },
          { label: '续办引用', value: body.continuationReference ?? '' },
          { label: '补偿续办引用', value: body.compensationReference ?? '' },
        ]}
      />
    </NoticeCard>
  );
}
