import { formatInstant } from '../moment';
import { useState } from 'react';
import { ConfirmDialog } from '@idpxyz/ui-primitives';
import { pendingText } from '../../components/action-feedback';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import {
  cancelParcel,
  type ApiResult,
  type CancellationDraft,
  type CancellationResponseBody,
} from './api';
import { cancellationOutcomeViews, cancellationPendingReasonViews, type OutcomeView } from './presentation';
import { DetailRow, Field, inputClass, NoticeCard, Section } from './controls';
import { ParcelNo } from './refs';
import { ResultPanel } from './ResultPanel';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';

// 取消运单(UC-PS-006 接受后取消入口):取消已接受委托中的明确包裹;货已收寄的包裹不能取消,转收寄后服务处置。
//
// 端点 POST /shipment-requests/parcel-cancellations 已真挂载(cmd/parcel-api 装配取消编排),本页照 withdraw 页模式
// 对形状接线:api.ts 收编端点形状、presentation 收编结果词表、表单只收客户可声明部分——来源信封与请求方身份由
// 接入适配器从认证结果铸造,业务发生时间由渠道契约声明,页面都不送。授权规则未登记时服务端答未决 +
// AUTHORITY_UNCONFIGURED,按未配置态如实呈现,不画成错误。
//
// 批量语义(UC-PS-006 步骤 1):端点一次受理一件包裹,批量只归组、不拥有共同状态。页面把多件包裹逐件分发、
// 逐件呈现结果,部分成功由此自然表达;一件的结果不回滚其他件已合法形成的决定。已收寄包裹这里绝不提供改回
// 「已取消」的口子:边界后的走向是待处置或明确拒绝,由服务端裁决,页面不预判。
//
// 屏上的话按运营的说法:「运单号」是委托标识,「客户单号」是原提交的来源请求键;边界规则不摆在表单上讲,
// 结果卡里按答案说。出处:parcel-shipment CONTEXT.md「包裹取消决定」「收寄后服务处置决定」与 UC-PS-006 决定矩阵。

interface CancellationFormState {
  originalRequestKey: string;
  shipmentRequestId: string;
  /** 每行一件包裹号;逐件分发时按行序受理。 */
  parcelIds: string;
  requesterReference: string;
  reasonReference: string;
}

const emptyForm: CancellationFormState = {
  originalRequestKey: '',
  shipmentRequestId: '',
  parcelIds: '',
  requesterReference: '',
  reasonReference: '',
};

/** 一件包裹的裁决结果,逐件并存(AT-PS-078:部分成功不以整单状态覆盖成员差异)。 */
interface ParcelCancellationEntry {
  parcelId: string;
  result: ApiResult<CancellationResponseBody>;
}

// 同一件包裹在一轮里只发一次:重复行不构成第二次请求语义(同一来源身份加包裹是
// 幂等键,重发只会得到 EXISTING_RESULT),去重保序。
function parcelIdsOf(form: CancellationFormState): string[] {
  return Array.from(
    new Set(
      form.parcelIds
        .split('\n')
        .map((line) => line.trim())
        .filter((line) => line !== ''),
    ),
  );
}

// 必填集取 UC-PS-006 取消请求的最低业务语义:目标委托、目标包裹、请求方与原因
// 缺一即构造不出取消请求(服务端同判 REQUEST_NOT_ACCEPTED)。授权判定属服务端,页面不判。
function validate(form: CancellationFormState): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!form.shipmentRequestId.trim()) errors.shipmentRequestId = '请填写运单号。';
  if (!form.originalRequestKey.trim()) errors.originalRequestKey = '请填写客户单号。';
  if (parcelIdsOf(form).length === 0) errors.parcelIds = '至少填写一个包裹号。';
  if (!form.requesterReference.trim()) errors.requesterReference = '请填写是谁提出取消。';
  if (!form.reasonReference.trim()) errors.reasonReference = '请填写取消原因。';
  return errors;
}

function buildDraft(form: CancellationFormState, parcelId: string): CancellationDraft {
  return {
    originalRequestKey: form.originalRequestKey.trim(),
    shipmentRequestId: form.shipmentRequestId.trim(),
    parcelId,
    requesterReference: form.requesterReference.trim(),
    reasonReference: form.reasonReference.trim(),
  };
}

// 确认弹层的正文(票 admin-web-workspace-form/05 第 3 条,蓝图 21.2):逐件列出要取消的包裹、影响什么、以谁的名义,
// 不写「确定吗」。边界句取 UC-PS-006 的口径:逐件裁决允许部分成功;已收寄的包裹不回退为「已取消」,转货物处置。
function cancellationConfirmText(form: CancellationFormState): string {
  const parcels = parcelIdsOf(form);
  return (
    `运单 ${form.shipmentRequestId.trim()}(客户单号 ${form.originalRequestKey.trim()})里的 ${parcels.length} 件包裹将逐件申请取消:` +
    `${parcels.join('、')}。每件单独判,可能只取消一部分;已经收寄的包裹取消不了,会转到货物处置。` +
    `申请人 ${form.requesterReference.trim()},原因:${form.reasonReference.trim()}。`
  );
}

export function CancelParcelPage() {
  const [form, setForm] = useState<CancellationFormState>(emptyForm);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [entries, setEntries] = useState<ParcelCancellationEntry[] | null>(null);

  const setField = (field: keyof CancellationFormState) => (value: string) =>
    setForm((current) => ({ ...current, [field]: value }));

  // 先校验再开确认:确认层问的是「要不要取消这几件」,不是「填全了没」——两问挤在一层,没填全会被读成取消被拒。
  function requestCancel() {
    const found = validate(form);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      setEntries(null);
      return;
    }
    setConfirming(true);
  }

  async function handleCancel() {
    setConfirming(false);
    const parcels = parcelIdsOf(form);
    setPending(true);
    setEntries([]);
    try {
      // 顺序逐件受理:每件独立请求独立作答,先答的结果当场入列,后件失败不动前件。
      for (const parcelId of parcels) {
        const result = await cancelParcel(buildDraft(form, parcelId));
        setEntries((current) => [...(current ?? []), { parcelId, result }]);
      }
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-[720px] mx-auto px-6 py-6 space-y-4">
        <header>
          <h1 className="text-[18px] font-bold text-idpxyz-textBright">取消运单</h1>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-1">
            客户要停一票已受理的运单时,在这里按包裹逐件申请取消。货已经收寄的包裹取消不了,会转到「货物处置」由运营决定去向;
            还没受理的运单要停请走「提交与撤回」。
          </p>
        </header>

        <Section title="哪一票运单">
          <div className="grid grid-cols-2 gap-3">
            <Field label="运单号" required error={errors.shipmentRequestId} hint="受理时给客户的运单号。">
              <input
                className={`${inputClass} font-mono`}
                value={form.shipmentRequestId}
                onChange={(event) => setField('shipmentRequestId')(event.target.value)}
              />
            </Field>
            <Field label="客户单号" required error={errors.originalRequestKey} hint="客户下单时自己的单号;与运单号一起才能找到这票运单。">
              <input
                className={`${inputClass} font-mono`}
                value={form.originalRequestKey}
                onChange={(event) => setField('originalRequestKey')(event.target.value)}
              />
            </Field>
          </div>
        </Section>

        <Section title="取消哪些包裹">
          <div className="space-y-3">
            <Field label="包裹号" required error={errors.parcelIds} hint="每行一个。每件单独判,可能只取消一部分。">
              <textarea
                className={`${inputClass} min-h-[72px] font-mono`}
                rows={3}
                value={form.parcelIds}
                onChange={(event) => setField('parcelIds')(event.target.value)}
              />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="谁提出取消" required error={errors.requesterReference} hint="客户或联系人的名字 / 编号,记在取消记录里。">
                <input
                  className={inputClass}
                  value={form.requesterReference}
                  onChange={(event) => setField('requesterReference')(event.target.value)}
                />
              </Field>
              <Field label="取消原因" required error={errors.reasonReference} hint="一句话说明,记在取消记录里。">
                <input
                  className={inputClass}
                  value={form.reasonReference}
                  onChange={(event) => setField('reasonReference')(event.target.value)}
                />
              </Field>
            </div>
          </div>
        </Section>

        <div className="flex items-center gap-3">
          <button
            type="button"
            disabled={pending}
            onClick={requestCancel}
            className="rounded border border-idpxyz-accent px-4 py-2 text-[13px] font-bold text-idpxyz-accent hover:bg-idpxyz-hover disabled:opacity-50"
          >
            {pending ? pendingText('取消') : '取消列出的包裹'}
          </button>
          {Object.keys(errors).length > 0 ? (
            <span className="text-[12px] text-idpxyz-textBright">还有没填好的地方,已在字段旁标出。</span>
          ) : null}
        </div>

        {entries !== null ? (
          <div className="space-y-3">
            {entries.map(({ parcelId, result }) => (
              <div key={parcelId} className="space-y-1">
                <div className="text-[12px] text-idpxyz-textMuted">
                  包裹 <ParcelNo parcelId={parcelId} className="text-idpxyz-text" />
                </div>
                <ResultPanel
                  result={result}
                  pending={false}
                  renderOutcome={(body, status) => <CancellationOutcomeCard body={body} status={status} />}
                />
              </div>
            ))}
            {pending ? (
              <div className="rounded border border-idpxyz-border p-4 text-[13px] text-idpxyz-textMuted">正在逐件提交…</div>
            ) : null}
          </div>
        ) : null}

        {/* 取消包裹是高风险动作,按下之后再拦一道;确认即逐件发请求,进行中与逐件结果仍由上面的按钮与逐件卡呈现。 */}
        <ConfirmDialog
          renderFooter={confirmDialogFooter}
          open={confirming}
          tone="danger"
          title="取消这些包裹"
          message={cancellationConfirmText(form)}
          confirmLabel="确认取消"
          cancelLabel="先不取消"
          onConfirm={handleCancel}
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

function CancellationOutcomeCard({ body, status }: { body: CancellationResponseBody; status: number }) {
  const view = cancellationOutcomeViews[body.outcome] ?? unknownOutcome;
  const pendingView = body.pendingReason ? cancellationPendingReasonViews[body.pendingReason] : undefined;
  return (
    <NoticeCard title={view.label} titleClass={view.affirmative ? 'text-idpxyz-accent' : 'text-idpxyz-textBright'}>
      <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">{view.note}</p>

      <dl className="mt-3 space-y-1">
        {body.decidedAt ? <DetailRow label="决定时间" value={formatInstant(body.decidedAt)} /> : null}
        {body.pendingReason ? <DetailRow label="未决原因" value={pendingView?.label ?? body.pendingReason} /> : null}
      </dl>

      {pendingView ? (
        // 未配置态用正文色而非沉底色:它是要人去登记参数的行动项,不是可忽略的细节;
        // 但仍不是错误色——未配置不是故障,也不是业务否定。
        <p className={`text-[11px] leading-4 mt-2 ${pendingView.unconfigured ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'}`}>
          {pendingView.note}
        </p>
      ) : null}

      {body.handoffReference ? (
        <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-2">取消已经成立,只是通知还没发出去;再提交一次只会重发通知,不会形成第二份取消。</p>
      ) : null}

      <TechnicalDetails
        rows={[
          { label: 'HTTP 状态', value: String(status) },
          { label: '结果码', value: body.outcome },
          { label: '取消决定标识', value: body.cancellationId ?? '' },
          { label: '越过的收寄版本', value: body.intakeVersion ?? '' },
          { label: '规则依据', value: body.refusalBasis ?? '' },
          { label: '既有终局', value: body.standingFinal ?? '' },
          { label: '未决原因码', value: body.pendingReason ?? '' },
          { label: '续办引用', value: body.continuationReference ?? '' },
          { label: '重发引用', value: body.handoffReference ?? '' },
        ]}
      />
    </NoticeCard>
  );
}
