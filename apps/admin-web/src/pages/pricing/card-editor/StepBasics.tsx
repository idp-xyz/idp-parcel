import { aggregationLabels, priceCardPurposeLabels, priceDirectionLabels } from '../../../vocabulary/price-card';
import { field } from '../card-form/fields';
import { defaultBaseChargeCode, type BasicsState } from '../card-form/model';
import { currencyChoices } from '../card-form/options';
import { allowedValues, sheetNames } from '../card-form/spec';
import { SelectField, TextField, SectionTitle, hintClass } from './controls';
import { useEditor } from './EditorContext';
import { labelled, weightUnitOptions } from './options';

// 方向与用途一一对应（领域里首发就是这三对）；选了方向顺手带出用途，运营仍可改，配对对不对由服务端判。
const pairedPurpose: Readonly<Record<string, string>> = {
  SELL: 'CUSTOMER_CHARGE',
  BUY: 'SUPPLIER_COST',
  INTERNAL: 'INTERNAL_PRICE',
};

export function StepBasics() {
  const { state, update, spec, env } = useEditor();
  const { basics } = state;
  const patch = (change: Partial<BasicsState>) => update((draft) => ({ ...draft, basics: { ...draft.basics, ...change } }));
  const purposes = allowedValues(spec, sheetNames.card, 'purpose');
  const currencies = currencyChoices(spec, env.currencies);

  return (
    <div className="space-y-6">
      <section>
        <SectionTitle hint="这张价卡叫什么、给谁用。方案标识加版本在系统里唯一。">方案</SectionTitle>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <TextField id={field.basics('planId')} label="方案标识" required mono value={basics.planId} onChange={(planId) => patch({ planId })} hint="如 CN-SG-EXPRESS-SELL，一经使用不要改" />
          <TextField id={field.basics('planVersion')} label="版本" required mono value={basics.planVersion} onChange={(planVersion) => patch({ planVersion })} hint="如 v1；调价时新建下一版" />
          <TextField id={field.basics('scope')} label="适用范围" required mono value={basics.scope} onChange={(scope) => patch({ scope })} hint="这张价卡用于哪一块业务，填已有的范围标识" />
          <SelectField
            id={field.basics('direction')}
            label="价格方向"
            required
            value={basics.direction}
            options={labelled(allowedValues(spec, sheetNames.card, 'direction'), priceDirectionLabels)}
            onChange={(direction) => {
              const purpose = pairedPurpose[direction];
              patch({
                direction,
                purpose: purpose !== undefined && purposes.includes(purpose) && (basics.purpose === '' || basics.purpose === pairedPurpose[basics.direction]) ? purpose : basics.purpose,
              });
            }}
            hint="售价是对客户收的，成本是供应商收我们的"
          />
          <SelectField
            id={field.basics('purpose')}
            label="用途"
            required
            value={basics.purpose}
            options={labelled(purposes, priceCardPurposeLabels)}
            onChange={(purpose) => patch({ purpose })}
          />
          <SelectField
            id={field.basics('aggregation')}
            label="计价对象"
            required
            value={basics.aggregation}
            options={labelled(allowedValues(spec, sheetNames.card, 'aggregation'), aggregationLabels)}
            onChange={(aggregation) => patch({ aggregation })}
            hint="选「每件计价」就是按每个包裹算钱，必须明确选，不替你默认"
          />
        </div>
      </section>

      <section>
        <SectionTitle hint="整张价卡的金额用同一种币种、重量用同一种单位。">币种与重量单位</SectionTitle>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {currencies.options !== null ? (
            <SelectField
              id={field.basics('currency')}
              label="币种"
              required
              value={basics.currency}
              options={currencies.options}
              onChange={(currency) => patch({ currency })}
            />
          ) : (
            <TextField
              id={field.basics('currency')}
              label="币种"
              required
              mono
              value={basics.currency}
              onChange={(currency) => patch({ currency: currency.toUpperCase() })}
              hint="币种目录暂时读不到，请直接填三位币种代码，如 CNY"
            />
          )}
          <SelectField
            id={field.basics('weightUnit')}
            label="重量单位"
            required
            value={basics.weightUnit}
            options={weightUnitOptions(spec, env)}
            onChange={(weightUnit) => patch({ weightUnit })}
          />
        </div>
      </section>

      <section>
        <SectionTitle hint="只填日期按当天零点（UTC）算；要写明时区就填完整时刻，如 2026-01-01T00:00:00+08:00。">生效时间</SectionTitle>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <TextField id={field.basics('periodStartsAt')} label="生效起点" required mono placeholder="2026-01-01" value={basics.periodStartsAt} onChange={(periodStartsAt) => patch({ periodStartsAt })} />
          <TextField id={field.basics('periodEndsAt')} label="生效止点" mono placeholder="不填即长期有效" value={basics.periodEndsAt} onChange={(periodEndsAt) => patch({ periodEndsAt })} />
        </div>
      </section>

      <section>
        <SectionTitle hint="价格方向要有商务授权背书，填授权的标识和版本。">价格方向授权</SectionTitle>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <TextField id={field.basics('authId')} label="授权标识" required mono value={basics.authId} onChange={(authId) => patch({ authId })} />
          <TextField id={field.basics('authVersion')} label="授权版本" required mono value={basics.authVersion} onChange={(authVersion) => patch({ authVersion })} />
        </div>
      </section>

      <details className="rounded border border-idpxyz-border px-3 py-2">
        <summary className="cursor-pointer select-none text-[12px] text-idpxyz-breadcrumb">更多设置</summary>
        <div className="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2">
          <TextField
            id={field.basics('baseChargeCode')}
            label="基础运费的费用代码"
            mono
            value={basics.baseChargeCode}
            onChange={(baseChargeCode) => patch({ baseChargeCode: baseChargeCode.toUpperCase() })}
            hint={`账单上基础运费这一行的代码，一般不用改（默认 ${defaultBaseChargeCode}）`}
          />
        </div>
        <p className={`${hintClass} mt-2`}>大写字母、数字和下划线。</p>
      </details>
    </div>
  );
}
