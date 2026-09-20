import { useState } from 'react';
import { pendingText } from '../../components/action-feedback';
import {
  submitShipmentRequest,
  type ApiResult,
  type DeclaredParcelDraft,
  type ShipmentRequestDraft,
  type SubmitResponseBody,
} from './api';
import {
  admissionControlLabels,
  authorityLabels,
  submitOutcomeViews,
  withCode,
  type OutcomeView,
} from './presentation';
import { DetailRow, Field, inputClass, NoticeCard, Section } from './controls';
import { ResultPanel } from './ResultPanel';

// UC-PS-001 提交表单页。字段只覆盖输入语义契约里「客户可声明」的各组;来源信封
// (租户、客户账户、来源、请求标识、两类时间)整组客户不可声明,只能来自认证结果,
// 所以页面上没有它们——放出来就是替接入适配器造信封。
//
// 校验只做形状级(必填与格式),必填的依据是文档或领域构造期就要求在场的东西;
// 真实产品的业务必填、枚举与跨字段条件属 PAR-COM-05/06 与适用 PAR-CUS-*,未登记
// 前一律不预设。

interface HeadFormState {
  customerShipmentReference: string;
  requestedServiceProduct: string;
  requestEffectiveAt: string;
  senderRelation: string;
  senderAddress: string;
  recipientRelation: string;
  recipientAddress: string;
  destinationServiceScope: string;
}

interface ParcelFormState {
  customerParcelReference: string;
  declaredWeightValue: string;
  declaredWeightUnit: string;
  declaredLength: string;
  declaredWidth: string;
  declaredHeight: string;
  declaredDimensionsUnit: string;
  goodsDescription: string;
  quantity: string;
  declaredValue: string;
  currency: string;
  originCountry: string;
  serviceNotes: string;
}

const emptyHead: HeadFormState = {
  customerShipmentReference: '',
  requestedServiceProduct: '',
  requestEffectiveAt: '',
  senderRelation: '',
  senderAddress: '',
  recipientRelation: '',
  recipientAddress: '',
  destinationServiceScope: '',
};

const emptyParcel: ParcelFormState = {
  customerParcelReference: '',
  declaredWeightValue: '',
  declaredWeightUnit: '',
  declaredLength: '',
  declaredWidth: '',
  declaredHeight: '',
  declaredDimensionsUnit: '',
  goodsDescription: '',
  quantity: '',
  declaredValue: '',
  currency: '',
  originCountry: '',
  serviceNotes: '',
};

// 镜像领域 MeasurementValue 的保真纪律:正十进制、点两侧都要有数字、不收指数与
// 符号;"0"/"0.00" 不是正数。页面先按同一形状拦,免得一来一回才学到同一句话。
function positiveDecimal(raw: string): boolean {
  return /^\d+(\.\d+)?$/.test(raw) && /[1-9]/.test(raw);
}

function positiveInteger(raw: string): boolean {
  return /^[1-9]\d*$/.test(raw);
}

function validate(head: HeadFormState, parcels: ParcelFormState[]): Record<string, string> {
  const errors: Record<string, string> = {};
  const requireText = (key: string, value: string) => {
    if (!value.trim()) errors[key] = '必填';
  };

  requireText('customerShipmentReference', head.customerShipmentReference);
  requireText('requestedServiceProduct', head.requestedServiceProduct);
  requireText('senderRelation', head.senderRelation);
  requireText('senderAddress', head.senderAddress);
  requireText('recipientRelation', head.recipientRelation);
  requireText('recipientAddress', head.recipientAddress);
  requireText('destinationServiceScope', head.destinationServiceScope);

  parcels.forEach((parcel, index) => {
    const key = (field: string) => `p${index}.${field}`;
    requireText(key('customerParcelReference'), parcel.customerParcelReference);

    // 毛重必备是领域构造期形状(没有重量的测量装配不出任何估价输入),不是业务默认。
    if (!parcel.declaredWeightValue.trim()) {
      errors[key('declaredWeightValue')] = '必填';
    } else if (!positiveDecimal(parcel.declaredWeightValue.trim())) {
      errors[key('declaredWeightValue')] = '须为正十进制数,如 1.50';
    }
    requireText(key('declaredWeightUnit'), parcel.declaredWeightUnit);

    // 外廓全有或全无:半截外廓在领域构造期即死,页面按同一条拦。
    const dims = [
      ['declaredLength', parcel.declaredLength],
      ['declaredWidth', parcel.declaredWidth],
      ['declaredHeight', parcel.declaredHeight],
      ['declaredDimensionsUnit', parcel.declaredDimensionsUnit],
    ] as const;
    const filled = dims.filter(([, value]) => value.trim() !== '');
    if (filled.length > 0 && filled.length < dims.length) {
      for (const [field, value] of dims) {
        if (!value.trim()) errors[key(field)] = '外廓须长、宽、高与单位齐全,或整体留空';
      }
    }
    for (const field of ['declaredLength', 'declaredWidth', 'declaredHeight'] as const) {
      const value = parcel[field].trim();
      if (value && !positiveDecimal(value)) {
        errors[key(field)] = '须为正十进制数';
      }
    }

    if (parcel.quantity.trim() && !positiveInteger(parcel.quantity.trim())) {
      errors[key('quantity')] = '须为正整数';
    }
    if (parcel.declaredValue.trim() && !positiveDecimal(parcel.declaredValue.trim())) {
      errors[key('declaredValue')] = '须为正十进制数';
    }
    if (parcel.currency.trim() && !/^[A-Za-z]{3}$/.test(parcel.currency.trim())) {
      errors[key('currency')] = '须为 3 个字母的币种代码';
    }
  });

  return errors;
}

function buildDraft(head: HeadFormState, parcels: ParcelFormState[]): ShipmentRequestDraft {
  const optional = (value: string) => {
    const trimmed = value.trim();
    return trimmed === '' ? undefined : trimmed;
  };
  return {
    customerShipmentReference: head.customerShipmentReference.trim(),
    requestedServiceProduct: head.requestedServiceProduct.trim(),
    // 留空即键缺席:requestEffectiveAt 的缺失/显式存在本身进入内容摘要,
    // 送空串会把「没说」变成「说了个空话」。
    requestEffectiveAt: optional(head.requestEffectiveAt),
    senderRelation: head.senderRelation.trim(),
    senderAddress: head.senderAddress.trim(),
    recipientRelation: head.recipientRelation.trim(),
    recipientAddress: head.recipientAddress.trim(),
    destinationServiceScope: head.destinationServiceScope.trim(),
    parcels: parcels.map<DeclaredParcelDraft>((parcel) => ({
      customerParcelReference: parcel.customerParcelReference.trim(),
      declaredWeightValue: parcel.declaredWeightValue.trim(),
      declaredWeightUnit: parcel.declaredWeightUnit.trim(),
      declaredLength: optional(parcel.declaredLength),
      declaredWidth: optional(parcel.declaredWidth),
      declaredHeight: optional(parcel.declaredHeight),
      declaredDimensionsUnit: optional(parcel.declaredDimensionsUnit),
      goodsDescription: optional(parcel.goodsDescription),
      quantity: optional(parcel.quantity),
      declaredValue: optional(parcel.declaredValue),
      currency: optional(parcel.currency),
      originCountry: optional(parcel.originCountry),
      serviceNotes: optional(parcel.serviceNotes),
    })),
  };
}

export function SubmitShipmentRequestPage() {
  const [head, setHead] = useState<HeadFormState>(emptyHead);
  const [parcels, setParcels] = useState<ParcelFormState[]>([{ ...emptyParcel }]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const [result, setResult] = useState<ApiResult<SubmitResponseBody> | null>(null);

  const setHeadField = (field: keyof HeadFormState) => (value: string) =>
    setHead((current) => ({ ...current, [field]: value }));

  const setParcelField = (index: number, field: keyof ParcelFormState) => (value: string) =>
    setParcels((current) =>
      current.map((parcel, at) => (at === index ? { ...parcel, [field]: value } : parcel)),
    );

  async function handleSubmit() {
    const found = validate(head, parcels);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      setResult(null);
      return;
    }
    setPending(true);
    try {
      setResult(await submitShipmentRequest(buildDraft(head, parcels)));
    } finally {
      setPending(false);
    }
  }

  const errorCount = Object.keys(errors).length;

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-[880px] mx-auto px-6 py-6 space-y-4">
        <header>
          <h1 className="text-[18px] font-bold text-idpxyz-textBright">
            提交国际小包服务请求
          </h1>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-1">
            提交动作不代表运营企业已经接受:委托建立后记录为「已提交」,接受或拒绝由
            接受判断流程另行形成(UC-PS-001)。
          </p>
        </header>

        <Section title="服务请求">
          <div className="grid grid-cols-2 gap-3">
            <Field
              label="客户委托参考"
              required
              error={errors.customerShipmentReference}
            >
              <input
                className={inputClass}
                value={head.customerShipmentReference}
                onChange={(event) => setHeadField('customerShipmentReference')(event.target.value)}
              />
            </Field>
            <Field
              label="请求的服务产品或服务要求"
              required
              error={errors.requestedServiceProduct}
              hint="真实服务产品目录尚未登记,此处按客户话语原样填写,不提供预设选项。"
            >
              <input
                className={inputClass}
                value={head.requestedServiceProduct}
                onChange={(event) => setHeadField('requestedServiceProduct')(event.target.value)}
              />
            </Field>
            <Field
              label="客户请求生效时间(requestEffectiveAt)"
              hint="留空表示「未提供」;该缺失本身参与重复与冲突判定,不会被默认成当前时间。"
            >
              <input
                type="datetime-local"
                className={inputClass}
                value={head.requestEffectiveAt}
                onChange={(event) => setHeadField('requestEffectiveAt')(event.target.value)}
              />
            </Field>
          </div>
        </Section>

        <Section title="寄收件范围">
          <div className="grid grid-cols-2 gap-3">
            <Field label="寄件关系" required error={errors.senderRelation}>
              <input
                className={inputClass}
                value={head.senderRelation}
                onChange={(event) => setHeadField('senderRelation')(event.target.value)}
              />
            </Field>
            <Field label="收件关系" required error={errors.recipientRelation}>
              <input
                className={inputClass}
                value={head.recipientRelation}
                onChange={(event) => setHeadField('recipientRelation')(event.target.value)}
              />
            </Field>
            <Field label="寄件地址" required error={errors.senderAddress}>
              <textarea
                className={`${inputClass} min-h-[56px]`}
                value={head.senderAddress}
                onChange={(event) => setHeadField('senderAddress')(event.target.value)}
              />
            </Field>
            <Field label="收件地址" required error={errors.recipientAddress}>
              <textarea
                className={`${inputClass} min-h-[56px]`}
                value={head.recipientAddress}
                onChange={(event) => setHeadField('recipientAddress')(event.target.value)}
              />
            </Field>
            <Field
              label="目的服务范围"
              required
              error={errors.destinationServiceScope}
            >
              <input
                className={inputClass}
                value={head.destinationServiceScope}
                onChange={(event) => setHeadField('destinationServiceScope')(event.target.value)}
              />
            </Field>
          </div>
          <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-3">
            同一委托的全部声明成员共享寄收件关系与目的服务范围;需要不同寄收件范围时
            请分成多份委托分别提交。
          </p>
        </Section>

        <Section title="声明包裹">
          <div className="space-y-3">
            {parcels.map((parcel, index) => (
              <ParcelCard
                key={index}
                index={index}
                parcel={parcel}
                errors={errors}
                onChange={setParcelField}
                onRemove={() =>
                  setParcels((current) => current.filter((_, at) => at !== index))
                }
                removable={parcels.length > 1}
              />
            ))}
          </div>
          <button
            type="button"
            className="mt-3 rounded border border-idpxyz-border px-3 py-1.5 text-[12px] text-idpxyz-text hover:bg-idpxyz-hover"
            onClick={() => setParcels((current) => [...current, { ...emptyParcel }])}
          >
            添加声明包裹
          </button>
          <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-2">
            每份委托必须至少包含一个包裹;整份委托一起判断,不支持成员级部分接受。
          </p>
        </Section>

        <div className="flex items-center gap-3">
          <button
            type="button"
            disabled={pending}
            onClick={handleSubmit}
            className="rounded border border-idpxyz-accent px-4 py-2 text-[13px] font-bold text-idpxyz-accent hover:bg-idpxyz-hover disabled:opacity-50"
          >
            {pending ? pendingText('提交') : '确认提交服务请求'}
          </button>
          {errorCount > 0 ? (
            <span className="text-[12px] text-idpxyz-textBright">
              尚有 {errorCount} 处输入不完整或格式不符,已在字段旁标出。
            </span>
          ) : null}
        </div>

        <ResultPanel
          result={result}
          pending={pending}
          renderOutcome={(body, status) => <SubmitOutcomeCard body={body} status={status} />}
        />
      </div>
    </div>
  );
}

function ParcelCard({
  index,
  parcel,
  errors,
  onChange,
  onRemove,
  removable,
}: {
  index: number;
  parcel: ParcelFormState;
  errors: Record<string, string>;
  onChange: (index: number, field: keyof ParcelFormState) => (value: string) => void;
  onRemove: () => void;
  removable: boolean;
}) {
  const error = (field: string) => errors[`p${index}.${field}`];
  const bind = (field: keyof ParcelFormState) => ({
    className: inputClass,
    value: parcel[field],
    onChange: (event: { target: { value: string } }) =>
      onChange(index, field)(event.target.value),
  });

  return (
    <div className="rounded border border-idpxyz-border p-3">
      <div className="flex items-center justify-between mb-3">
        <span className="text-[13px] font-bold text-idpxyz-textBright">
          声明包裹 #{index + 1}
        </span>
        <button
          type="button"
          disabled={!removable}
          onClick={onRemove}
          title={removable ? undefined : '每份委托必须至少包含一个包裹'}
          className="rounded border border-idpxyz-border px-2 py-1 text-[11px] text-idpxyz-textMuted hover:bg-idpxyz-hover disabled:opacity-40"
        >
          移除
        </button>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <Field
          label="客户侧包裹引用"
          required
          error={error('customerParcelReference')}
        >
          <input {...bind('customerParcelReference')} />
        </Field>
      </div>

      <p className="text-[12px] text-idpxyz-textMuted mt-3 mb-2">声明测量</p>
      <div className="grid grid-cols-4 gap-3">
        <Field
          label="毛重数值"
          required
          error={error('declaredWeightValue')}
          hint="原样保全,如 1.50 不会被改写成 1.5。"
        >
          <input {...bind('declaredWeightValue')} inputMode="decimal" />
        </Field>
        <Field label="毛重单位" required error={error('declaredWeightUnit')}>
          <input {...bind('declaredWeightUnit')} placeholder="单位引用,如 KG" />
        </Field>
      </div>
      <div className="grid grid-cols-4 gap-3 mt-2">
        <Field label="外廓·长" error={error('declaredLength')}>
          <input {...bind('declaredLength')} inputMode="decimal" />
        </Field>
        <Field label="外廓·宽" error={error('declaredWidth')}>
          <input {...bind('declaredWidth')} inputMode="decimal" />
        </Field>
        <Field label="外廓·高" error={error('declaredHeight')}>
          <input {...bind('declaredHeight')} inputMode="decimal" />
        </Field>
        <Field label="外廓单位" error={error('declaredDimensionsUnit')}>
          <input {...bind('declaredDimensionsUnit')} placeholder="如 CM" />
        </Field>
      </div>
      <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-1">
        外廓可整体不报;报则长、宽、高与单位须齐全。
      </p>

      <p className="text-[12px] text-idpxyz-textMuted mt-3 mb-2">
        申报原始资料
        <span className="ml-2 text-[11px]">
          具体必填字段由真实产品和关务区域决定,此处不预设必填。
        </span>
      </p>
      <div className="grid grid-cols-2 gap-3">
        <Field label="品名" error={error('goodsDescription')}>
          <input {...bind('goodsDescription')} />
        </Field>
        <Field label="数量" error={error('quantity')}>
          <input {...bind('quantity')} inputMode="numeric" />
        </Field>
        <Field label="价值" error={error('declaredValue')}>
          <input {...bind('declaredValue')} inputMode="decimal" />
        </Field>
        <Field label="币种" error={error('currency')}>
          <input {...bind('currency')} placeholder="三个字母,如 USD" />
        </Field>
        <Field label="原产地" error={error('originCountry')}>
          <input {...bind('originCountry')} />
        </Field>
        <Field label="货物与服务资料补充" error={error('serviceNotes')}>
          <input {...bind('serviceNotes')} />
        </Field>
      </div>
    </div>
  );
}

// 未收录的 outcome 原词展示,不译不并:词汇表是封闭的,页面落后于服务端时如实
// 报「不认识」,比错并进某个认识的格要安全。
const unknownOutcome: OutcomeView = {
  label: '',
  note: '本页尚未收录该结果词,请以原词核对服务端记录与用例文档。',
};

function SubmitOutcomeCard({ body, status }: { body: SubmitResponseBody; status: number }) {
  const view = submitOutcomeViews[body.outcome] ?? unknownOutcome;
  const ownership = body.productionOwnership;
  return (
    <NoticeCard
      title={withCode(view.label, body.outcome)}
      titleClass={view.affirmative ? 'text-idpxyz-accent' : 'text-idpxyz-textBright'}
      meta={`HTTP ${status}${status === 201 ? ' · 新建成立' : ''}`}
    >
      <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">{view.note}</p>

      <dl className="mt-3 space-y-1">
        {body.shipmentRequestId ? (
          <DetailRow label="委托标识" value={body.shipmentRequestId} mono />
        ) : null}
      </dl>

      {ownership ? (
        <div className="mt-3 rounded border border-idpxyz-border p-3">
          <p className="text-[12px] font-bold text-idpxyz-textBright mb-2">生产归属决定</p>
          <dl className="space-y-1">
            <DetailRow label="判断标识" value={ownership.decisionId} mono />
            <DetailRow
              label="权威身份"
              value={withCode(authorityLabels[ownership.authority], ownership.authority)}
            />
            <DetailRow
              label="准入控制"
              value={withCode(
                admissionControlLabels[ownership.admissionControl],
                ownership.admissionControl,
              )}
            />
            <DetailRow label="规则版本" value={ownership.ruleVersion} mono />
            <DetailRow label="修订" value={ownership.revision} mono />
            {ownership.otherAuthority ? (
              <DetailRow label="当前权威方" value={ownership.otherAuthority} mono />
            ) : null}
            {ownership.handoffReference ? (
              <DetailRow label="交接引用" value={ownership.handoffReference} mono />
            ) : null}
            {ownership.unresolvedReason ? (
              <DetailRow label="未决原因" value={ownership.unresolvedReason} mono />
            ) : null}
            {ownership.continuationReference ? (
              <DetailRow label="续办引用" value={ownership.continuationReference} mono />
            ) : null}
            {ownership.suspensionReference ? (
              <DetailRow label="暂停引用" value={ownership.suspensionReference} mono />
            ) : null}
          </dl>
        </div>
      ) : null}

      {body.gateBlockReasons && body.gateBlockReasons.length > 0 ? (
        <div className="mt-3">
          <p className="text-[12px] text-idpxyz-textMuted mb-1">未来提交闸门阻断原因</p>
          <ul className="space-y-0.5">
            {body.gateBlockReasons.map((reason) => (
              <li key={reason} className="text-[12px] font-mono text-idpxyz-text">
                {reason}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </NoticeCard>
  );
}
