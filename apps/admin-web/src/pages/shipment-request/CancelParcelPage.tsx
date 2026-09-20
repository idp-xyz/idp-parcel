import { useState } from 'react';
import { ConfirmDialog } from '@idpxyz/ui-primitives';
import { pendingText } from '../../components/action-feedback';
import {
  cancelParcel,
  type ApiResult,
  type CancellationDraft,
  type CancellationResponseBody,
} from './api';
import {
  cancellationOutcomeViews,
  cancellationPendingReasonViews,
  withCode,
  type OutcomeView,
} from './presentation';
import { DetailRow, Field, inputClass, NoticeCard, Section } from './controls';
import { ResultPanel } from './ResultPanel';

// UC-PS-006 接受后取消入口:取消已接受委托中的明确包裹,或协调收寄后服务处置。
//
// 端点 POST /shipment-requests/parcel-cancellations 已真挂载(cmd/parcel-api 装配
// 取消编排),本页照 withdraw 页模式对形状接线:api.ts 收编端点形状、presentation
// 收编结果词表、表单只收客户可声明部分——来源信封与请求方身份由接入适配器从认证
// 结果铸造,业务发生时间由渠道契约声明,页面都不送。授权规则未登记时服务端答
// 未决 + AUTHORITY_UNCONFIGURED,按未配置态如实呈现,不画成错误。
//
// 批量语义(UC-PS-006 步骤 1):端点一次受理一件包裹,批量只归组、不拥有共同状态。
// 页面把多件包裹逐件分发、逐件呈现结果,部分成功由此自然表达;一件的结果不回滚
// 其他件已合法形成的决定。已收寄包裹这里绝不提供改回「已取消」的口子:边界后的
// 走向是待处置或明确拒绝,由服务端裁决,页面不预判。
//
// 边界文案的出处:parcel-shipment CONTEXT.md「包裹取消决定」「收寄后服务处置决定」
// 与 UC-PS-006 决定矩阵。措辞守住该用例的「本用例不执行」:不把客户消息自动解释
// 为取消。

interface CancellationFormState {
  originalRequestKey: string;
  shipmentRequestId: string;
  /** 每行一件包裹标识;逐件分发时按行序受理。 */
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
// 缺一即构造不出取消请求(服务端同判 REQUEST_NOT_ACCEPTED)。授权判定属服务端,
// 页面不判。
function validate(form: CancellationFormState): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!form.originalRequestKey.trim()) errors.originalRequestKey = '必填';
  if (!form.shipmentRequestId.trim()) errors.shipmentRequestId = '必填';
  if (parcelIdsOf(form).length === 0) errors.parcelIds = '至少填写一件包裹标识';
  if (!form.requesterReference.trim()) errors.requesterReference = '必填';
  if (!form.reasonReference.trim()) errors.reasonReference = '必填';
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

// 确认弹层的正文（票 admin-web-workspace-form/05 第 3 条，蓝图 21.2）：逐件列出要取消的包裹标识、影响什么、以谁的名义,
// 不写「确定吗」。边界句取 UC-PS-006 的口径：逐件裁决允许部分成功;越过取消边界的不回退为「已取消」,转收寄后服务处置。
function cancellationConfirmText(form: CancellationFormState): string {
  const parcels = parcelIdsOf(form);
  return (
    `将对委托 ${form.shipmentRequestId.trim()}(来源请求 ${form.originalRequestKey.trim()})中的 ${parcels.length} 件包裹逐件请求取消:` +
    `${parcels.join('、')}。每件独立裁决,允许部分成功;已越过取消边界的包裹不回退为「已取消」,转收寄后服务处置。` +
    `以请求方 ${form.requesterReference.trim()} 的名义,原因 ${form.reasonReference.trim()}。`
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
          <h1 className="text-[18px] font-bold text-idpxyz-textBright">
            取消包裹或协调收寄后服务处置
          </h1>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-1">
            对已接受委托中的明确包裹逐件裁决(UC-PS-006):尚未越过适用取消边界时形成
            包裹取消决定;已越过边界时明确拒绝回退取消,转收寄后服务处置。仍为
            「已提交」的整份委托撤回走「决定前撤回」,不在本页。
          </p>
        </header>

        <Section title="入口边界">
          <ul className="list-disc pl-5 space-y-1.5 text-[12px] leading-5 text-idpxyz-textMuted">
            <li>
              取消权按包裹判断,批量请求允许部分成功;一个包裹的结果不回滚其他包裹
              已合法形成的决定。
            </li>
            <li>
              网络服务包裹只允许在有效网络收寄结果形成前取消;有效网络收寄已先行成立时
              不回退为「已取消」,只能形成收寄后服务处置决定、保持未决或明确拒绝。
            </li>
            <li>
              取消保留包裹身份、接受基线、请求、授权与既有交易或作业历史;已有面单交易时,
              包裹取消不能代替渠道作废或渠道退款结果。
            </li>
            <li>
              客户消息、异常信号、拒收、无路由或运输停止本身不自动生成取消、退运或
              服务终止;处置决定必须由有权主体明确形成。
            </li>
          </ul>
        </Section>

        <Section title="目标委托">
          <div className="grid grid-cols-2 gap-3">
            <Field
              label="原提交的来源请求标识"
              required
              error={errors.originalRequestKey}
              hint="产生该委托的那次提交所用的请求标识;与委托标识双重指名,缺一即统一不可见。"
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
          </div>
        </Section>

        <Section title="取消请求">
          <div className="space-y-3">
            <Field
              label="目标包裹标识"
              required
              error={errors.parcelIds}
              hint="每行一件。取消按包裹逐件裁决,多件时逐件发出请求、逐件返回结果,允许部分成功。"
            >
              <textarea
                className={`${inputClass} min-h-[72px] font-mono`}
                rows={3}
                value={form.parcelIds}
                onChange={(event) => setField('parcelIds')(event.target.value)}
              />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field
                label="请求方引用"
                required
                error={errors.requesterReference}
                hint="取消授权的真实规则待租户登记,此处只记录引用,授权由服务端逐件判定。"
              >
                <input
                  className={inputClass}
                  value={form.requesterReference}
                  onChange={(event) => setField('requesterReference')(event.target.value)}
                />
              </Field>
              <Field
                label="取消原因引用"
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
          </div>
        </Section>

        <div className="flex items-center gap-3">
          <button
            type="button"
            disabled={pending}
            onClick={requestCancel}
            className="rounded border border-idpxyz-accent px-4 py-2 text-[13px] font-bold text-idpxyz-accent hover:bg-idpxyz-hover disabled:opacity-50"
          >
            {pending ? pendingText('取消') : '逐件请求取消列出的包裹'}
          </button>
          {Object.keys(errors).length > 0 ? (
            <span className="text-[12px] text-idpxyz-textBright">
              尚有输入不完整,已在字段旁标出。
            </span>
          ) : null}
        </div>

        {entries !== null ? (
          <div className="space-y-3">
            {entries.map(({ parcelId, result }) => (
              <div key={parcelId} className="space-y-1">
                <div className="text-[12px] text-idpxyz-textMuted">
                  包裹 <span className="font-mono text-idpxyz-text">{parcelId}</span>
                </div>
                <ResultPanel
                  result={result}
                  pending={false}
                  renderOutcome={(body, status) => (
                    <CancellationOutcomeCard body={body} status={status} />
                  )}
                />
              </div>
            ))}
            {pending ? (
              <div className="rounded border border-idpxyz-border p-4 text-[13px] text-idpxyz-textMuted">
                正在逐件请求,等待服务端答复…
              </div>
            ) : null}
          </div>
        ) : null}

        {/* 取消包裹是高风险动作,按下之后再拦一道;确认即逐件发请求,进行中与逐件结果仍由上面的按钮与逐件卡呈现。 */}
        <ConfirmDialog
          open={confirming}
          tone="danger"
          title="逐件请求取消列出的包裹"
          message={cancellationConfirmText(form)}
          confirmLabel="逐件请求取消"
          cancelLabel="不取消"
          onConfirm={handleCancel}
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

function CancellationOutcomeCard({
  body,
  status,
}: {
  body: CancellationResponseBody;
  status: number;
}) {
  const view = cancellationOutcomeViews[body.outcome] ?? unknownOutcome;
  const pendingView = body.pendingReason
    ? cancellationPendingReasonViews[body.pendingReason]
    : undefined;
  return (
    <NoticeCard
      title={withCode(view.label, body.outcome)}
      titleClass={view.affirmative ? 'text-idpxyz-accent' : 'text-idpxyz-textBright'}
      meta={`HTTP ${status}${status === 201 ? ' · 取消新建成立' : ''}`}
    >
      <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">{view.note}</p>

      <dl className="mt-3 space-y-1">
        {body.cancellationId ? (
          <DetailRow label="取消决定标识" value={body.cancellationId} mono />
        ) : null}
        {body.decidedAt ? <DetailRow label="判断时间" value={body.decidedAt} mono /> : null}
        {body.intakeVersion ? (
          <DetailRow label="越过的收寄版本" value={body.intakeVersion} mono />
        ) : null}
        {body.refusalBasis ? (
          <DetailRow label="规则依据" value={body.refusalBasis} mono />
        ) : null}
        {body.pendingReason ? (
          <DetailRow
            label="未决原因"
            value={withCode(pendingView?.label, body.pendingReason)}
          />
        ) : null}
        {body.continuationReference ? (
          <DetailRow label="续办引用" value={body.continuationReference} mono />
        ) : null}
        {body.handoffReference ? (
          <DetailRow label="重发引用" value={body.handoffReference} mono />
        ) : null}
      </dl>

      {pendingView ? (
        // 未配置态用正文色而非沉底色:它是要人去登记参数的行动项,不是可忽略的细节;
        // 但仍不是错误色——未配置不是故障,也不是业务否定。
        <p
          className={`text-[11px] leading-4 mt-2 ${
            pendingView.unconfigured ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'
          }`}
        >
          {pendingView.note}
        </p>
      ) : null}

      {body.handoffReference ? (
        <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-2">
          重发引用与判断续办是两条路:取消决定已成立,只是发布意图未交出;重放同一
          请求会重发同一份意图,不会形成第二份决定。
        </p>
      ) : null}
    </NoticeCard>
  );
}
