import { contentCategoryOptions, type GoodsLineForm } from './goods-form';
import { Field, inputClass } from './controls';

// 委托级货物明细的录入(ADR-0373,票 declared-goods-lines/05),小包下单页与平台货件页共用。一行一种货;几件包裹共用这一份,
// 每件装了哪几行在装箱矩阵里填。哪些格必填由服务产品的接单规则说,这里一格都不标必填。
//
// 增删行交给调用方:去掉一行时装箱矩阵要跟着去掉那一列,这份同步属于页面。

export function GoodsLinesEditor({
  lines,
  contentCategory,
  contentCategoryNote,
  errors,
  onChangeLine,
  onAddLine,
  onRemoveLine,
  onChangeCategory,
  onChangeCategoryNote,
}: {
  lines: GoodsLineForm[];
  contentCategory: string;
  contentCategoryNote: string;
  errors: Record<string, string>;
  onChangeLine: (index: number, field: keyof GoodsLineForm, value: string) => void;
  onAddLine: () => void;
  onRemoveLine: (index: number) => void;
  onChangeCategory: (value: string) => void;
  onChangeCategoryNote: (value: string) => void;
}) {
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <Field label="内件性质" error={errors.contentCategory} hint="照报关单上的勾选填;文件类可以不填货物行。">
          <select className={inputClass} value={contentCategory} onChange={(event) => onChangeCategory(event.target.value)}>
            <option value="">不填</option>
            {contentCategoryOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </Field>
        {contentCategory === 'OTHER' ? (
          <Field label="说明" hint="「其他」是什么,一句话。">
            <input className={inputClass} value={contentCategoryNote} onChange={(event) => onChangeCategoryNote(event.target.value)} />
          </Field>
        ) : null}
      </div>

      {lines.map((line, index) => (
        <GoodsLineCard
          key={index}
          index={index}
          line={line}
          errors={errors}
          onChange={(field, value) => onChangeLine(index, field, value)}
          onRemove={() => onRemoveLine(index)}
        />
      ))}
      <button
        type="button"
        className="rounded border border-idpxyz-border px-3 py-1.5 type-body text-idpxyz-text hover:bg-idpxyz-hover"
        onClick={onAddLine}
      >
        再加一行货
      </button>
      <p className="type-caption leading-4 text-idpxyz-textMuted">
        一行一种货,相当于商业发票的一行。值照客户给的填,系统不换算、不核对单价乘数量;一格都没填的行不提交。
      </p>
    </div>
  );
}

function GoodsLineCard({
  index,
  line,
  errors,
  onChange,
  onRemove,
}: {
  index: number;
  line: GoodsLineForm;
  errors: Record<string, string>;
  onChange: (field: keyof GoodsLineForm, value: string) => void;
  onRemove: () => void;
}) {
  const bind = (field: keyof GoodsLineForm) => ({
    className: inputClass,
    value: line[field],
    onChange: (event: { target: { value: string } }) => onChange(field, event.target.value),
  });
  const error = (field: keyof GoodsLineForm) => errors[`g${index}.${field}`];

  return (
    <div className="rounded border border-idpxyz-border p-3">
      <div className="flex items-center justify-between mb-2">
        <span className="type-value font-bold text-idpxyz-textBright">第 {index + 1} 行</span>
        <button
          type="button"
          onClick={onRemove}
          aria-label={`去掉第 ${index + 1} 行货`}
          className="rounded border border-idpxyz-border px-2 py-1 type-caption text-idpxyz-textMuted hover:bg-idpxyz-hover"
        >
          去掉这行
        </button>
      </div>
      <div className="grid grid-cols-4 gap-3">
        <div className="col-span-2">
          <Field label="中文品名">
            <input {...bind('descriptionZh')} />
          </Field>
        </div>
        <div className="col-span-2">
          <Field label="英文品名">
            <input {...bind('descriptionEn')} />
          </Field>
        </div>
        <Field label="数量" error={error('quantity')}>
          <input {...bind('quantity')} inputMode="decimal" />
        </Field>
        <Field label="单位">
          <input {...bind('unit')} placeholder="如 PCS" />
        </Field>
        <Field label="单价" error={error('unitValue')}>
          <input {...bind('unitValue')} inputMode="decimal" />
        </Field>
        <Field label="行总价" error={error('lineValue')}>
          <input {...bind('lineValue')} inputMode="decimal" />
        </Field>
        <Field label="币种">
          <input {...bind('currency')} placeholder="如 USD" />
        </Field>
        <Field label="净重" error={error('netWeight')} hint="整行的净重。">
          <input {...bind('netWeight')} inputMode="decimal" />
        </Field>
        <Field label="净重单位">
          <input {...bind('netWeightUnit')} placeholder="如 KG" />
        </Field>
        <Field label="原产国">
          <input {...bind('originCountry')} placeholder="如 CN" />
        </Field>
        <Field label="出口编码">
          <input {...bind('exportCode')} />
        </Field>
        <Field label="进口编码">
          <input {...bind('importCode')} />
        </Field>
        <Field label="SKU" hint="货主自己的商品编码。">
          <input {...bind('sku')} />
        </Field>
        <div />
        <Field label="销售平台">
          <input {...bind('orderPlatform')} placeholder="如 SHOPIFY" />
        </Field>
        <Field label="销售订单号">
          <input {...bind('orderNumber')} />
        </Field>
        <Field label="订单行">
          <input {...bind('orderLine')} />
        </Field>
      </div>
    </div>
  );
}
