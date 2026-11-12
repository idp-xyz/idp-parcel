import { useState } from 'react';
import { pendingText } from '../../components/action-feedback';
import { submitShipmentRequest, type ApiResult, type DeclaredParcelDraft, type ShipmentRequestDraft, type SubmitResponseBody } from './api';
import { emptyGoodsLine, goodsDraftOfLines, validateGoodsLines, type GoodsLineForm } from './goods-form';
import {
  packingDraftsOf,
  packingErrorsOf,
  withoutBlankGoodsLines,
  withoutPackingBox,
  withoutPackingLine,
  type PackingMatrix,
} from './packing-form';
import { duplicatedAt, positiveDecimal, requestEffectiveAtOf } from './form-shapes';
import { objectNouns } from '../../vocabulary/objects';
import { currentDisplayTimeZone, zoneLabel } from '../moment';
import { Field, inputClass, Section } from './controls';
import { GoodsLinesEditor } from './GoodsLinesEditor';
import { PackingMatrixEditor } from './PackingMatrixEditor';
import { SubmitOutcomeCard } from './SubmitOutcomeCard';
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
  serviceNotes: '',
};

/** 货物明细是委托级的(ADR-0373):几件包裹共用一份货物行;多件时可选报每件装了哪几行。 */
interface GoodsFormState {
  lines: GoodsLineForm[];
  contentCategory: string;
  contentCategoryNote: string;
  /** 多件委托报不报装箱分配;单件委托不出现这一项,一件就是全部。 */
  reportPacking: boolean;
  packing: PackingMatrix;
}

const emptyGoods: GoodsFormState = {
  lines: [{ ...emptyGoodsLine }],
  contentCategory: '',
  contentCategoryNote: '',
  reportPacking: false,
  packing: [],
};

function packingReported(goods: GoodsFormState, parcels: ParcelFormState[]): boolean {
  return goods.reportPacking && parcels.length > 1;
}

const required = '请填写';

function validate(head: HeadFormState, parcels: ParcelFormState[], goods: GoodsFormState): Record<string, string> {
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
  if (requestEffectiveAtOf(head.requestEffectiveAt) === null) errors.requestEffectiveAt = '日期和时刻要填全';

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
  });

  // 客户包裹号在一票里要各不相同:服务端按它认件,两件同号必拒。
  for (const index of duplicatedAt(parcels.map((parcel) => parcel.customerParcelReference))) {
    errors[`p${index}.customerParcelReference`] = '与另一件的客户包裹号重了';
  }

  Object.assign(errors, validateGoodsLines(goods.lines, goods.contentCategory));
  if (packingReported(goods, parcels)) Object.assign(errors, packingErrorsOf(goods.lines, goods.packing, parcels.length));
  return errors;
}

function buildDraft(head: HeadFormState, parcels: ParcelFormState[], goods: GoodsFormState): ShipmentRequestDraft {
  const optional = (value: string) => {
    const trimmed = value.trim();
    return trimmed === '' ? undefined : trimmed;
  };
  const packing = packingReported(goods, parcels) ? packingDraftsOf(goods.lines, goods.packing, parcels.length) : undefined;
  return {
    customerShipmentReference: head.customerShipmentReference.trim(),
    requestedServiceProduct: head.requestedServiceProduct.trim(),
    // 留空即键缺席:requestEffectiveAt 的缺失/显式存在本身进入内容摘要,
    // 送空串会把「没说」变成「说了个空话」。服务端按 RFC 3339 读,墙钟按显示时区折过去。
    requestEffectiveAt: requestEffectiveAtOf(head.requestEffectiveAt) ?? undefined,
    senderRelation: head.senderRelation.trim(),
    senderAddress: head.senderAddress.trim(),
    recipientRelation: head.recipientRelation.trim(),
    recipientAddress: head.recipientAddress.trim(),
    destinationServiceScope: head.destinationServiceScope.trim(),
    parcels: parcels.map<DeclaredParcelDraft>((parcel, index) => ({
      customerParcelReference: parcel.customerParcelReference.trim(),
      declaredWeightValue: parcel.declaredWeightValue.trim(),
      declaredWeightUnit: parcel.declaredWeightUnit.trim(),
      declaredLength: optional(parcel.declaredLength),
      declaredWidth: optional(parcel.declaredWidth),
      declaredHeight: optional(parcel.declaredHeight),
      declaredDimensionsUnit: optional(parcel.declaredDimensionsUnit),
      serviceNotes: optional(parcel.serviceNotes),
      ...(packing ? { packing: packing[index] } : {}),
    })),
    goods: goodsDraftOfLines(goods.lines, goods.contentCategory, goods.contentCategoryNote),
  };
}

export function SubmitShipmentRequestPage() {
  const [head, setHead] = useState<HeadFormState>(emptyHead);
  const [parcels, setParcels] = useState<ParcelFormState[]>([{ ...emptyParcel }]);
  const [goods, setGoods] = useState<GoodsFormState>(emptyGoods);
  // 错误在第一次提交之后随表单现算,不存快照:包裹与货物行都按位置编键,去掉一件后存下的快照会把错挂到别的件上。
  const [attempted, setAttempted] = useState(false);
  const [pending, setPending] = useState(false);
  const [result, setResult] = useState<ApiResult<SubmitResponseBody> | null>(null);
  const errors = attempted ? validate(head, parcels, goods) : {};

  const setHeadField = (field: keyof HeadFormState) => (value: string) => setHead((current) => ({ ...current, [field]: value }));

  const setParcelField = (index: number, field: keyof ParcelFormState) => (value: string) =>
    setParcels((current) => current.map((parcel, at) => (at === index ? { ...parcel, [field]: value } : parcel)));

  const removeParcel = (index: number) => {
    setParcels((current) => current.filter((_, at) => at !== index));
    setGoods((current) => ({ ...current, packing: withoutPackingBox(current.packing, index) }));
  };
  const setGoodsLine = (index: number, field: keyof GoodsLineForm, value: string) =>
    setGoods((current) => ({ ...current, lines: current.lines.map((line, at) => (at === index ? { ...line, [field]: value } : line)) }));
  const removeGoodsLine = (index: number) =>
    setGoods((current) => ({
      ...current,
      lines: current.lines.filter((_, at) => at !== index),
      packing: withoutPackingLine(current.packing, index),
    }));

  async function handleSubmit() {
    // 先收掉空行:服务端的行号只数非空行,收掉之后屏上的「第几行」与答复里的才是同一行。
    const compacted = withoutBlankGoodsLines(goods.lines, goods.packing);
    const nextGoods = { ...goods, lines: compacted.lines, packing: compacted.matrix };
    setGoods(nextGoods);
    setAttempted(true);
    if (Object.keys(validate(head, parcels, nextGoods)).length > 0) {
      setResult(null);
      return;
    }
    setPending(true);
    try {
      setResult(await submitShipmentRequest(buildDraft(head, parcels, nextGoods)));
    } finally {
      setPending(false);
    }
  }

  const errorCount = Object.keys(errors).length;

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-[880px] mx-auto px-6 py-6 space-y-4">
        <header>
          <h1 className="type-title font-bold text-idpxyz-textBright">新建运单</h1>
          <p className="type-body leading-5 text-idpxyz-textMuted mt-1">
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
            <Field
              label="客户要求的开始时间"
              error={errors.requestEffectiveAt}
              hint={`客户没说就留空,不要填现在;按 ${zoneLabel(currentDisplayTimeZone(), new Date())} 读。`}
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
          <p className="type-caption leading-4 text-idpxyz-textMuted mt-3">一票运单里的包裹寄收件人和目的地相同;寄收件不同的货请分开建运单。</p>
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
                onRemove={() => removeParcel(index)}
                removable={parcels.length > 1}
              />
            ))}
          </div>
          <button
            type="button"
            className="mt-3 rounded border border-idpxyz-border px-3 py-1.5 type-body text-idpxyz-text hover:bg-idpxyz-hover"
            onClick={() => setParcels((current) => [...current, { ...emptyParcel }])}
          >
            再加一件
          </button>
          <p className="type-caption leading-4 text-idpxyz-textMuted mt-2">一票运单至少一件包裹;整票一起受理或拒绝,不会只收一部分。</p>
        </Section>

        <Section title={objectNouns.goodsDetails}>
          <GoodsLinesEditor
            lines={goods.lines}
            contentCategory={goods.contentCategory}
            contentCategoryNote={goods.contentCategoryNote}
            errors={errors}
            onChangeLine={setGoodsLine}
            onAddLine={() => setGoods((current) => ({ ...current, lines: [...current.lines, { ...emptyGoodsLine }] }))}
            onRemoveLine={removeGoodsLine}
            onChangeCategory={(value) => setGoods((current) => ({ ...current, contentCategory: value }))}
            onChangeCategoryNote={(value) => setGoods((current) => ({ ...current, contentCategoryNote: value }))}
          />
          {parcels.length > 1 ? (
            <div className="mt-4 space-y-3">
              <label className="flex items-center gap-2 type-body text-idpxyz-text">
                <input
                  type="checkbox"
                  checked={goods.reportPacking}
                  onChange={(event) => setGoods((current) => ({ ...current, reportPacking: event.target.checked }))}
                />
                报每件装了哪几行
              </label>
              <p className="type-caption leading-4 text-idpxyz-textMuted">
                不报就是整票一份清单,只有发票没有箱单时就这样交。报就每件都报、每行各件加起来等于这一行的数量。
              </p>
              {goods.reportPacking ? (
                <PackingMatrixEditor
                  lines={goods.lines}
                  boxNames={parcels.map((parcel, index) => parcel.customerParcelReference.trim() || `第 ${index + 1} 件`)}
                  matrix={goods.packing}
                  onChange={(packing) => setGoods((current) => ({ ...current, packing }))}
                />
              ) : null}
            </div>
          ) : null}
        </Section>

        <div className="flex items-center gap-3">
          <button
            type="button"
            disabled={pending}
            onClick={handleSubmit}
            className="rounded border border-idpxyz-accent px-4 py-2 type-value font-bold text-idpxyz-accent hover:bg-idpxyz-hover disabled:opacity-50"
          >
            {pending ? pendingText('提交') : '提交运单'}
          </button>
          {errorCount > 0 ? <span className="type-body text-idpxyz-textBright">还有 {errorCount} 处没填好,已在字段旁标出。</span> : null}
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
        <span className="type-value font-bold text-idpxyz-textBright">第 {index + 1} 件</span>
        <button
          type="button"
          disabled={!removable}
          onClick={onRemove}
          title={removable ? undefined : '至少要有一件包裹'}
          aria-label={`去掉第 ${index + 1} 件`}
          className="rounded border border-idpxyz-border px-2 py-1 type-caption text-idpxyz-textMuted hover:bg-idpxyz-hover disabled:opacity-40"
        >
          去掉这件
        </button>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <Field label="客户包裹号" required error={error('customerParcelReference')} hint="客户给这件包裹的编号。">
          <input {...bind('customerParcelReference')} />
        </Field>
      </div>

      <p className="type-body text-idpxyz-textMuted mt-3 mb-2">重量与尺寸</p>
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
      <p className="type-caption leading-4 text-idpxyz-textMuted mt-1">尺寸可以不填;填就长、宽、高和单位一起填。</p>

      <div className="grid grid-cols-2 gap-3 mt-3">
        <Field label="备注" error={error('serviceNotes')}>
          <input {...bind('serviceNotes')} />
        </Field>
      </div>
    </div>
  );
}
