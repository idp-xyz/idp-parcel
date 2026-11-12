import { useState } from 'react';
import { pendingText } from '../../components/action-feedback';
import { submitShipmentRequest, type ApiResult, type SubmitResponseBody } from './api';
import { emptyGoodsLine } from './goods-form';
import { withoutBlankGoodsLines, withoutPackingBox, withoutPackingLine } from './packing-form';
import {
  emptyFbaBox,
  emptyFbaForm,
  fbaDraftOf,
  validateFba,
  withMeasurementsFromFirstBox,
  withSequence,
  type FbaBoxForm,
  type FbaForm,
  type FbaHeadForm,
} from './fba-form';
import { Field, inputClass, Section } from './controls';
import { GoodsLinesEditor } from './GoodsLinesEditor';
import { PackingMatrixEditor } from './PackingMatrixEditor';
import { SubmitOutcomeCard } from './SubmitOutcomeCard';
import { ResultPanel } from './ResultPanel';
import { objectNouns } from '../../vocabulary/objects';
import { currentDisplayTimeZone, zoneLabel } from '../moment';

// 平台货件下单(ADR-0374 决定三:模型统一、入口按场景分;票 declared-goods-lines/05)。提交的是同一个 /v1/shipment-requests,
// 一个平台货件一份委托、一箱一个包裹;表单逻辑在 fba-form.ts,这里只排版。
//
// 平台货件号、箱标号不标必填:要不要必填由服务产品的接单规则说(决定三),页面只拦服务端必拒的形状。
//
// 错误在第一次提交之后随表单现算,不存快照:箱与行都按位置编键,删掉一箱后存下的快照会把错挂到别的箱上。

const smallButton =
  'rounded border border-idpxyz-border px-3 py-1.5 type-body text-idpxyz-text hover:bg-idpxyz-hover disabled:opacity-40';
const cellInput =
  'w-full min-w-[72px] rounded border bg-idpxyz-inputBg px-1.5 py-1 type-value text-idpxyz-text focus:outline-none focus:border-idpxyz-accent';

export function FbaShipmentPage() {
  const [form, setForm] = useState<FbaForm>(emptyFbaForm);
  const [attempted, setAttempted] = useState(false);
  const [pending, setPending] = useState(false);
  const [result, setResult] = useState<ApiResult<SubmitResponseBody> | null>(null);
  const errors = attempted ? validateFba(form) : {};

  const update = (change: (current: FbaForm) => Partial<FbaForm>) => setForm((current) => ({ ...current, ...change(current) }));
  const headInput = (field: keyof FbaHeadForm) => ({
    className: inputClass,
    value: form.head[field],
    onChange: (event: { target: { value: string } }) => {
      const value = event.target.value;
      update((current) => ({ head: { ...current.head, [field]: value } }));
    },
  });

  async function handleSubmit() {
    // 先收掉空行:服务端的行号只数非空行,收掉之后屏上的「第几行」与答复里的才是同一行。
    const compacted = withoutBlankGoodsLines(form.lines, form.packing);
    const next = { ...form, lines: compacted.lines, packing: compacted.matrix };
    setForm(next);
    setAttempted(true);
    if (Object.keys(validateFba(next)).length > 0) {
      setResult(null);
      return;
    }
    setPending(true);
    try {
      setResult(await submitShipmentRequest(fbaDraftOf(next)));
    } finally {
      setPending(false);
    }
  }

  const errorCount = Object.keys(errors).length;
  const zone = zoneLabel(currentDisplayTimeZone(), new Date());

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-[1080px] mx-auto px-6 py-6 space-y-4">
        <header>
          <h1 className="type-title font-bold text-idpxyz-textBright">新建平台货件</h1>
          <p className="type-body leading-5 text-idpxyz-textMuted mt-1">
            送往电商平台仓的货(如亚马逊 FBA):一个平台货件建一张运单,每箱是一件包裹,送往同一个平台仓。提交后和普通运单一样等受理;
            整柜、海卡与卡派不走这里。
          </p>
        </header>

        <Section title="运单信息">
          <div className="grid grid-cols-3 gap-3">
            <Field label={objectNouns.customerReference} required error={errors.customerShipmentReference}>
              <input {...headInput('customerShipmentReference')} />
            </Field>
            <Field label="服务产品" required error={errors.requestedServiceProduct} hint="照客户的说法填。">
              <input {...headInput('requestedServiceProduct')} />
            </Field>
            <Field label="客户要求的开始时间" error={errors.requestEffectiveAt} hint={`客户没说就留空;按 ${zone} 读。`}>
              <input type="datetime-local" {...headInput('requestEffectiveAt')} />
            </Field>
          </div>
        </Section>

        <Section title={objectNouns.platformShipment}>
          <div className="grid grid-cols-3 gap-3">
            <Field label={objectNouns.platform} error={errors.platform} hint="如 AMAZON,照客户的写法。">
              <input {...headInput('platform')} />
            </Field>
            <Field label={objectNouns.platformShipmentNumber} error={errors.shipmentId} hint="如亚马逊的 Shipment ID。">
              <input {...headInput('shipmentId')} className={`${inputClass} font-mono`} />
            </Field>
            <Field label={objectNouns.platformReferenceNumber} hint="如亚马逊的 Reference ID,没有就留空。">
              <input {...headInput('referenceId')} className={`${inputClass} font-mono`} />
            </Field>
          </div>
          <p className="type-caption leading-4 text-idpxyz-textMuted mt-2">号由客户交来,系统不向平台核对;要不要必填由服务产品的接单规则定。</p>
        </Section>

        <Section title="发货与平台仓">
          <div className="grid grid-cols-2 gap-3">
            <Field label="发货人" required error={errors.senderRelation}>
              <input {...headInput('senderRelation')} />
            </Field>
            <Field label="平台仓代码" required error={errors.warehouseCode} hint="收件人就是平台仓,如 ONT8。">
              <input {...headInput('warehouseCode')} />
            </Field>
            <Field label="发货地址" required error={errors.senderAddress}>
              <textarea {...headInput('senderAddress')} className={`${inputClass} min-h-[56px]`} />
            </Field>
            <Field label="平台仓地址" required error={errors.warehouseAddress}>
              <textarea {...headInput('warehouseAddress')} className={`${inputClass} min-h-[56px]`} />
            </Field>
            <Field label="目的地" required error={errors.destinationServiceScope} hint="平台仓所在的国家或地区。">
              <input {...headInput('destinationServiceScope')} />
            </Field>
          </div>
        </Section>

        <Section title={`箱子(${form.boxes.length} 箱)`}>
          <BoxesEditor
            head={form.head}
            boxes={form.boxes}
            errors={errors}
            onHead={(field, value) => update((current) => ({ head: { ...current.head, [field]: value } }))}
            onBoxes={(change) => update((current) => ({ boxes: change(current.boxes) }))}
            onRemove={(index) =>
              update((current) => ({
                boxes: current.boxes.filter((_, at) => at !== index),
                packing: withoutPackingBox(current.packing, index),
              }))
            }
          />
        </Section>

        <Section title={objectNouns.goodsDetails}>
          <GoodsLinesEditor
            lines={form.lines}
            contentCategory={form.contentCategory}
            contentCategoryNote={form.contentCategoryNote}
            errors={errors}
            onChangeLine={(index, field, value) =>
              update((current) => ({ lines: current.lines.map((line, at) => (at === index ? { ...line, [field]: value } : line)) }))
            }
            onAddLine={() => update((current) => ({ lines: [...current.lines, { ...emptyGoodsLine }] }))}
            onRemoveLine={(index) =>
              update((current) => ({
                lines: current.lines.filter((_, at) => at !== index),
                packing: withoutPackingLine(current.packing, index),
              }))
            }
            onChangeCategory={(value) => update(() => ({ contentCategory: value }))}
            onChangeCategoryNote={(value) => update(() => ({ contentCategoryNote: value }))}
          />
        </Section>

        <Section title={objectNouns.packing}>
          <p className="type-caption leading-4 text-idpxyz-textMuted mb-3">
            每箱装了哪几行各多少。可以整张不填(整票一份清单);填就每箱都填、每行各箱加起来等于这一行的数量。
          </p>
          <PackingMatrixEditor
            lines={form.lines}
            boxNames={form.boxes.map(
              (box, index) => `${index + 1}. ${box.customerParcelReference.trim() || box.platformBoxLabel.trim() || '未填号'}`,
            )}
            matrix={form.packing}
            onChange={(packing) => update(() => ({ packing }))}
          />
        </Section>

        <div className="flex items-center gap-3">
          <button
            type="button"
            disabled={pending}
            onClick={handleSubmit}
            className="rounded border border-idpxyz-accent px-4 py-2 type-value font-bold text-idpxyz-accent hover:bg-idpxyz-hover disabled:opacity-50"
          >
            {pending ? pendingText('提交') : '提交货件'}
          </button>
          {errorCount > 0 ? <span className="type-body text-idpxyz-textBright">还有 {errorCount} 处没填好,已在字段旁或表下标出。</span> : null}
        </div>

        <ResultPanel result={result} pending={pending} renderOutcome={(body, status) => <SubmitOutcomeCard body={body} status={status} />} />
      </div>
    </div>
  );
}

const boxFieldLabels: Record<keyof FbaBoxForm, string> = {
  customerParcelReference: '客户包裹号',
  platformBoxLabel: objectNouns.platformBoxLabel,
  weight: '毛重',
  length: '长',
  width: '宽',
  height: '高',
};

function BoxesEditor({
  head,
  boxes,
  errors,
  onHead,
  onBoxes,
  onRemove,
}: {
  head: FbaHeadForm;
  boxes: FbaBoxForm[];
  errors: Record<string, string>;
  onHead: (field: 'weightUnit' | 'dimensionsUnit', value: string) => void;
  onBoxes: (change: (current: FbaBoxForm[]) => FbaBoxForm[]) => void;
  onRemove: (index: number) => void;
}) {
  const [addCount, setAddCount] = useState('1');
  const [sequence, setSequence] = useState({
    field: 'customerParcelReference' as 'customerParcelReference' | 'platformBoxLabel',
    prefix: '',
    start: '1',
    width: '3',
  });
  const count = Number(addCount);
  // 一次加几箱只设个防手滑的上限,不是件数规则:件数上限(若有)由服务端说。
  const canAdd = Number.isInteger(count) && count >= 1 && count <= 500;
  const start = Number(sequence.start);
  const width = Number(sequence.width);
  const canSequence =
    sequence.start.trim() !== '' && Number.isInteger(start) && start >= 0 && Number.isInteger(width) && width >= 1 && width <= 12;

  const cell = (index: number, field: keyof FbaBoxForm, mono = false) => {
    const error = errors[`b${index}.${field}`];
    return (
      <input
        className={`${cellInput} ${mono ? 'font-mono' : ''} ${error ? 'border-idpxyz-textBright' : 'border-idpxyz-border'}`}
        title={error}
        aria-label={`第 ${index + 1} 箱${boxFieldLabels[field]}`}
        aria-invalid={error ? true : undefined}
        value={boxes[index][field]}
        onChange={(event) => {
          const value = event.target.value;
          onBoxes((current) => current.map((box, at) => (at === index ? { ...box, [field]: value } : box)));
        }}
      />
    );
  };

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-4 gap-3">
        <Field label="重量单位" required error={errors.weightUnit} hint="整票的箱子用同一个单位。">
          <input className={inputClass} value={head.weightUnit} placeholder="如 KG" onChange={(event) => onHead('weightUnit', event.target.value)} />
        </Field>
        <Field label="尺寸单位" error={errors.dimensionsUnit} hint="填了尺寸就要填。">
          <input className={inputClass} value={head.dimensionsUnit} placeholder="如 CM" onChange={(event) => onHead('dimensionsUnit', event.target.value)} />
        </Field>
      </div>

      <div className="flex flex-wrap items-end gap-2 type-body text-idpxyz-textMuted">
        <span>按序号填</span>
        <select
          className={`${inputClass} w-auto`}
          aria-label="按序号填哪一列"
          value={sequence.field}
          onChange={(event) => setSequence((current) => ({ ...current, field: event.target.value as typeof current.field }))}
        >
          <option value="customerParcelReference">{boxFieldLabels.customerParcelReference}</option>
          <option value="platformBoxLabel">{boxFieldLabels.platformBoxLabel}</option>
        </select>
        <input
          className={`${inputClass} w-[200px] font-mono`}
          aria-label="序号前缀"
          placeholder="前缀,如 FBA15XXXXU"
          value={sequence.prefix}
          onChange={(event) => setSequence((current) => ({ ...current, prefix: event.target.value }))}
        />
        <span>起始</span>
        <input
          className={`${inputClass} w-[64px]`}
          inputMode="numeric"
          aria-label="起始序号"
          value={sequence.start}
          onChange={(event) => setSequence((current) => ({ ...current, start: event.target.value }))}
        />
        <span>位数</span>
        <input
          className={`${inputClass} w-[56px]`}
          inputMode="numeric"
          aria-label="序号位数"
          value={sequence.width}
          onChange={(event) => setSequence((current) => ({ ...current, width: event.target.value }))}
        />
        <button
          type="button"
          className={smallButton}
          disabled={!canSequence}
          onClick={() => onBoxes((current) => withSequence(current, sequence.field, sequence.prefix, start, width))}
        >
          填空格
        </button>
        <button type="button" className={smallButton} disabled={boxes.length < 2} onClick={() => onBoxes(withMeasurementsFromFirstBox)}>
          其余照第 1 箱填重量尺寸
        </button>
      </div>

      <div className="max-h-[480px] overflow-auto rounded border border-idpxyz-border">
        <table className="w-full border-collapse">
          <thead className="sticky top-0 z-10 bg-idpxyz-editor">
            <tr className="type-caption text-idpxyz-textMuted text-left">
              <th scope="col" className="px-2 py-1.5">箱</th>
              <th scope="col" className="px-2 py-1.5">{boxFieldLabels.customerParcelReference} *</th>
              <th scope="col" className="px-2 py-1.5">{boxFieldLabels.platformBoxLabel}</th>
              <th scope="col" className="px-2 py-1.5">{boxFieldLabels.weight} *</th>
              <th scope="col" className="px-2 py-1.5">{boxFieldLabels.length}</th>
              <th scope="col" className="px-2 py-1.5">{boxFieldLabels.width}</th>
              <th scope="col" className="px-2 py-1.5">{boxFieldLabels.height}</th>
              <th scope="col" className="px-2 py-1.5">
                <span className="sr-only">操作</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {boxes.map((_, index) => (
              <tr key={index} className="border-t border-idpxyz-border">
                <th scope="row" className="px-2 py-1 text-left type-body font-normal text-idpxyz-textMuted">
                  {index + 1}
                </th>
                <td className="px-2 py-1">{cell(index, 'customerParcelReference', true)}</td>
                <td className="px-2 py-1">{cell(index, 'platformBoxLabel', true)}</td>
                <td className="px-2 py-1">{cell(index, 'weight')}</td>
                <td className="px-2 py-1">{cell(index, 'length')}</td>
                <td className="px-2 py-1">{cell(index, 'width')}</td>
                <td className="px-2 py-1">{cell(index, 'height')}</td>
                <td className="px-2 py-1">
                  <button
                    type="button"
                    disabled={boxes.length <= 1}
                    title={boxes.length <= 1 ? '至少要有一箱' : undefined}
                    aria-label={`去掉第 ${index + 1} 箱`}
                    onClick={() => onRemove(index)}
                    className="rounded border border-idpxyz-border px-2 py-1 type-caption text-idpxyz-textMuted hover:bg-idpxyz-hover disabled:opacity-40"
                  >
                    去掉
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <BoxErrorSummary boxes={boxes.length} errors={errors} />

      <div className="flex items-center gap-2 type-body text-idpxyz-textMuted">
        <span>再加</span>
        <input
          className={`${inputClass} w-[64px]`}
          inputMode="numeric"
          aria-label="再加几箱"
          value={addCount}
          onChange={(event) => setAddCount(event.target.value)}
        />
        <span>箱</span>
        <button
          type="button"
          className={smallButton}
          disabled={!canAdd}
          onClick={() => onBoxes((current) => [...current, ...Array.from({ length: count }, () => ({ ...emptyFbaBox }))])}
        >
          加上
        </button>
      </div>
    </div>
  );
}

// 箱子表的格太窄放不下错误句,在表下按箱汇总。
function BoxErrorSummary({ boxes, errors }: { boxes: number; errors: Record<string, string> }) {
  const rows: string[] = [];
  for (let box = 0; box < boxes; box += 1) {
    const parts = Object.entries(boxFieldLabels).flatMap(([field, label]) => {
      const error = errors[`b${box}.${field}`];
      return error ? [`${label}${error === '请填写' ? '没填' : `:${error}`}`] : [];
    });
    if (parts.length > 0) rows.push(`第 ${box + 1} 箱 ${parts.join(',')}`);
  }
  if (rows.length === 0 && !errors.boxes) return null;
  return (
    <ul className="list-disc pl-5 type-caption leading-4 text-idpxyz-textBright space-y-0.5">
      {errors.boxes ? <li>{errors.boxes}</li> : null}
      {rows.slice(0, 10).map((row) => (
        <li key={row}>{row}</li>
      ))}
      {rows.length > 10 ? <li>另有 {rows.length - 10} 箱没填好</li> : null}
    </ul>
  );
}
