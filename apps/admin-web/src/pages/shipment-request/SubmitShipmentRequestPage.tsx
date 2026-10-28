import { useState } from 'react';
import { pendingText } from '../../components/action-feedback';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { submitShipmentRequest, type ApiResult, type DeclaredParcelDraft, type ShipmentRequestDraft, type SubmitResponseBody } from './api';
import { admissionControlLabels, authorityLabels, submitOutcomeViews, type OutcomeView } from './presentation';
import { DetailRow, Field, inputClass, NoticeCard, Section } from './controls';
import { ShipmentNo } from './refs';
import { ResultPanel } from './ResultPanel';

// 新建运单(UC-PS-001 提交表单页)。字段只覆盖输入语义契约里「客户可声明」的各组;来源信封
// (租户、客户账户、来源、请求标识、两类时间)整组客户不可声明,只能来自认证结果,
// 所以页面上没有它们——放出来就是替接入适配器造信封。
//
// 校验只做形状级(必填与格式),必填的依据是文档或领域构造期就要求在场的东西;
// 真实产品的业务必填、枚举与跨字段条件属 PAR-COM-05/06 与适用 PAR-CUS-*,未登记
// 前一律不预设。屏上的话按填单的说法:「客户单号」是客户委托参考,「包裹」是声明包裹。

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

const required = '请填写';

function validate(head: HeadFormState, parcels: ParcelFormState[]): Record<string, string> {
  const errors: Record<string, string> = {};
  const requireText = (key: string, value: string) => {
    if (!value.trim()) errors[key] = required;
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
      errors[key('declaredWeightValue')] = required;
    } else if (!positiveDecimal(parcel.declaredWeightValue.trim())) {
      errors[key('declaredWeightValue')] = '请填大于 0 的数,如 1.50';
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
        if (!value.trim()) errors[key(field)] = '尺寸要长、宽、高和单位一起填,或都不填';
      }
    }
    for (const field of ['declaredLength', 'declaredWidth', 'declaredHeight'] as const) {
      const value = parcel[field].trim();
      if (value && !positiveDecimal(value)) {
        errors[key(field)] = '请填大于 0 的数';
      }
    }

    if (parcel.quantity.trim() && !positiveInteger(parcel.quantity.trim())) {
      errors[key('quantity')] = '请填大于 0 的整数';
    }
    if (parcel.declaredValue.trim() && !positiveDecimal(parcel.declaredValue.trim())) {
      errors[key('declaredValue')] = '请填大于 0 的数';
    }
    if (parcel.currency.trim() && !/^[A-Za-z]{3}$/.test(parcel.currency.trim())) {
      errors[key('currency')] = '币种填 3 个字母,如 USD';
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

  const setHeadField = (field: keyof HeadFormState) => (value: string) => setHead((current) => ({ ...current, [field]: value }));

  const setParcelField = (index: number, field: keyof ParcelFormState) => (value: string) =>
    setParcels((current) => current.map((parcel, at) => (at === index ? { ...parcel, [field]: value } : parcel)));

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
          <h1 className="text-[18px] font-bold text-idpxyz-textBright">新建运单</h1>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-1">
            替客户录一票运单。提交后由系统判断能不能受理;受理之前它在运单列表里是「待受理」。
          </p>
        </header>

        <Section title="运单信息">
          <div className="grid grid-cols-2 gap-3">
            <Field label="客户单号" required error={errors.customerShipmentReference} hint="客户自己的单号,之后查运单、取消都要用它。">
              <input
                className={inputClass}
                value={head.customerShipmentReference}
                onChange={(event) => setHeadField('customerShipmentReference')(event.target.value)}
              />
            </Field>
            <Field label="服务产品" required error={errors.requestedServiceProduct} hint="客户要的服务,照客户的说法填。">
              <input
                className={inputClass}
                value={head.requestedServiceProduct}
                onChange={(event) => setHeadField('requestedServiceProduct')(event.target.value)}
              />
            </Field>
            <Field label="客户要求的开始时间" hint="客户没说就留空,不要填现在。">
              <input
                type="datetime-local"
                className={inputClass}
                value={head.requestEffectiveAt}
                onChange={(event) => setHeadField('requestEffectiveAt')(event.target.value)}
              />
            </Field>
          </div>
        </Section>

        <Section title="寄件与收件">
          <div className="grid grid-cols-2 gap-3">
            <Field label="寄件人" required error={errors.senderRelation}>
              <input className={inputClass} value={head.senderRelation} onChange={(event) => setHeadField('senderRelation')(event.target.value)} />
            </Field>
            <Field label="收件人" required error={errors.recipientRelation}>
              <input className={inputClass} value={head.recipientRelation} onChange={(event) => setHeadField('recipientRelation')(event.target.value)} />
            </Field>
            <Field label="寄件地址" required error={errors.senderAddress}>
              <textarea className={`${inputClass} min-h-[56px]`} value={head.senderAddress} onChange={(event) => setHeadField('senderAddress')(event.target.value)} />
            </Field>
            <Field label="收件地址" required error={errors.recipientAddress}>
              <textarea className={`${inputClass} min-h-[56px]`} value={head.recipientAddress} onChange={(event) => setHeadField('recipientAddress')(event.target.value)} />
            </Field>
            <Field label="目的地" required error={errors.destinationServiceScope} hint="送到哪个国家或地区。">
              <input className={inputClass} value={head.destinationServiceScope} onChange={(event) => setHeadField('destinationServiceScope')(event.target.value)} />
            </Field>
          </div>
          <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-3">一票运单里的包裹寄收件人和目的地相同;寄收件不同的货请分开建运单。</p>
        </Section>

        <Section title="包裹">
          <div className="space-y-3">
            {parcels.map((parcel, index) => (
              <ParcelCard
                key={index}
                index={index}
                parcel={parcel}
                errors={errors}
                onChange={setParcelField}
                onRemove={() => setParcels((current) => current.filter((_, at) => at !== index))}
                removable={parcels.length > 1}
              />
            ))}
          </div>
          <button
            type="button"
            className="mt-3 rounded border border-idpxyz-border px-3 py-1.5 text-[12px] text-idpxyz-text hover:bg-idpxyz-hover"
            onClick={() => setParcels((current) => [...current, { ...emptyParcel }])}
          >
            再加一件
          </button>
          <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-2">一票运单至少一件包裹;整票一起受理或拒绝,不会只收一部分。</p>
        </Section>

        <div className="flex items-center gap-3">
          <button
            type="button"
            disabled={pending}
            onClick={handleSubmit}
            className="rounded border border-idpxyz-accent px-4 py-2 text-[13px] font-bold text-idpxyz-accent hover:bg-idpxyz-hover disabled:opacity-50"
          >
            {pending ? pendingText('提交') : '提交运单'}
          </button>
          {errorCount > 0 ? <span className="text-[12px] text-idpxyz-textBright">还有 {errorCount} 处没填好,已在字段旁标出。</span> : null}
        </div>

        <ResultPanel result={result} pending={pending} renderOutcome={(body, status) => <SubmitOutcomeCard body={body} status={status} />} />
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
    onChange: (event: { target: { value: string } }) => onChange(index, field)(event.target.value),
  });

  return (
    <div className="rounded border border-idpxyz-border p-3">
      <div className="flex items-center justify-between mb-3">
        <span className="text-[13px] font-bold text-idpxyz-textBright">第 {index + 1} 件</span>
        <button
          type="button"
          disabled={!removable}
          onClick={onRemove}
          title={removable ? undefined : '至少要有一件包裹'}
          className="rounded border border-idpxyz-border px-2 py-1 text-[11px] text-idpxyz-textMuted hover:bg-idpxyz-hover disabled:opacity-40"
        >
          去掉这件
        </button>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <Field label="客户包裹号" required error={error('customerParcelReference')} hint="客户给这件包裹的编号。">
          <input {...bind('customerParcelReference')} />
        </Field>
      </div>

      <p className="text-[12px] text-idpxyz-textMuted mt-3 mb-2">重量与尺寸</p>
      <div className="grid grid-cols-4 gap-3">
        <Field label="毛重" required error={error('declaredWeightValue')} hint="照客户报的填,如 1.50。">
          <input {...bind('declaredWeightValue')} inputMode="decimal" />
        </Field>
        <Field label="重量单位" required error={error('declaredWeightUnit')}>
          <input {...bind('declaredWeightUnit')} placeholder="如 KG" />
        </Field>
      </div>
      <div className="grid grid-cols-4 gap-3 mt-2">
        <Field label="长" error={error('declaredLength')}>
          <input {...bind('declaredLength')} inputMode="decimal" />
        </Field>
        <Field label="宽" error={error('declaredWidth')}>
          <input {...bind('declaredWidth')} inputMode="decimal" />
        </Field>
        <Field label="高" error={error('declaredHeight')}>
          <input {...bind('declaredHeight')} inputMode="decimal" />
        </Field>
        <Field label="尺寸单位" error={error('declaredDimensionsUnit')}>
          <input {...bind('declaredDimensionsUnit')} placeholder="如 CM" />
        </Field>
      </div>
      <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-1">尺寸可以不填;填就长、宽、高和单位一起填。</p>

      <p className="text-[12px] text-idpxyz-textMuted mt-3 mb-2">申报资料</p>
      <div className="grid grid-cols-2 gap-3">
        <Field label="品名" error={error('goodsDescription')}>
          <input {...bind('goodsDescription')} />
        </Field>
        <Field label="数量" error={error('quantity')}>
          <input {...bind('quantity')} inputMode="numeric" />
        </Field>
        <Field label="申报价值" error={error('declaredValue')}>
          <input {...bind('declaredValue')} inputMode="decimal" />
        </Field>
        <Field label="币种" error={error('currency')}>
          <input {...bind('currency')} placeholder="如 USD" />
        </Field>
        <Field label="原产地" error={error('originCountry')}>
          <input {...bind('originCountry')} />
        </Field>
        <Field label="备注" error={error('serviceNotes')}>
          <input {...bind('serviceNotes')} />
        </Field>
      </div>
    </div>
  );
}

// 未收录的 outcome 如实报「不认识」,不译不并:词汇表是封闭的,页面落后于服务端时
// 比错并进某个认识的格要安全;原词在技术详情里。
const unknownOutcome: OutcomeView = {
  label: '结果未收录',
  note: '系统交回了本页还不认识的结果,请按技术详情里的结果码核对记录。',
};

function SubmitOutcomeCard({ body, status }: { body: SubmitResponseBody; status: number }) {
  const view = submitOutcomeViews[body.outcome] ?? unknownOutcome;
  const ownership = body.productionOwnership;
  return (
    <NoticeCard title={view.label} titleClass={view.affirmative ? 'text-idpxyz-accent' : 'text-idpxyz-textBright'}>
      <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">{view.note}</p>

      <dl className="mt-3 space-y-1">
        {body.shipmentRequestId ? (
          <div className="flex gap-2 text-[12px] leading-5">
            <dt className="text-idpxyz-textMuted shrink-0 w-[104px]">运单号</dt>
            <dd>
              <ShipmentNo shipmentRequestId={body.shipmentRequestId} />
            </dd>
          </div>
        ) : null}
        {ownership ? <DetailRow label="由谁承运" value={authorityLabels[ownership.authority] ?? ownership.authority} /> : null}
        {ownership && ownership.admissionControl !== 'OPEN' ? (
          <DetailRow label="接单状态" value={admissionControlLabels[ownership.admissionControl] ?? ownership.admissionControl} />
        ) : null}
      </dl>

      {body.gateBlockReasons && body.gateBlockReasons.length > 0 ? (
        <p className="text-[11px] leading-4 text-idpxyz-textMuted mt-2">之后再提交会被拦住,原因见技术详情。</p>
      ) : null}

      <TechnicalDetails
        rows={[
          { label: 'HTTP 状态', value: String(status) },
          { label: '结果码', value: body.outcome },
          { label: '委托标识', value: body.shipmentRequestId ?? '' },
          { label: '生产归属判断', value: ownership?.decisionId ?? '' },
          { label: '权威身份', value: ownership?.authority ?? '' },
          { label: '准入控制', value: ownership?.admissionControl ?? '' },
          { label: '规则版本', value: ownership?.ruleVersion ?? '' },
          { label: '修订', value: ownership?.revision ?? '' },
          { label: '当前权威方', value: ownership?.otherAuthority ?? '' },
          { label: '交接引用', value: ownership?.handoffReference ?? '' },
          { label: '未决原因', value: ownership?.unresolvedReason ?? '' },
          { label: '续办引用', value: ownership?.continuationReference ?? '' },
          { label: '交接确认引用', value: ownership?.handoffConfirmationReference ?? '' },
          { label: '暂停引用', value: ownership?.suspensionReference ?? '' },
          { label: '闸门阻断原因', value: (body.gateBlockReasons ?? []).join(' · ') },
        ]}
      />
    </NoticeCard>
  );
}
