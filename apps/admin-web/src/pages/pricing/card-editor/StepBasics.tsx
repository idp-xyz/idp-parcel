import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { accessRefusalTitle, isOperatorRefusal } from '../../../auth/access-refusal';
import { aggregationLabels, priceCardPurposeLabels, priceDirectionLabels } from '../../../vocabulary/price-card';
import { listServiceProducts } from '../../party/api';
import { serviceProductObjectOptions } from '../../party/picker-options';
import { useLoaded } from '../../party/PublicationFormFields';
import { field } from '../card-form/fields';
import { basicsWithDirection, defaultBaseChargeCode, type BasicsState } from '../card-form/model';
import { currencyChoices } from '../card-form/options';
import { allowedValues, sheetNames } from '../card-form/spec';
import { specTakesServiceProducts } from '../card-form/workbook';
import {
  addTypedServiceProducts,
  declaresServiceProducts,
  restProductsHint,
  serviceProductChoices,
  serviceProductsNotTakenNote,
  toggleServiceProduct,
} from '../service-products';
import { FieldBox, SelectField, TextField, SectionTitle, hintClass } from './controls';
import { useEditor } from './EditorContext';
import { labelled, weightUnitOptions } from './options';

/**
 * 适用服务产品：从服务产品册勾选，可多选、可不选（ADR-0378 决定一）。册读不到时退回按标识添加，并说清为什么——表单不因此
 * 变死；标识在不在册由服务端在预览时判。还在读册时只说在读，不先摆手填框：册一读回来手填框就换成勾选，先摆出来只会让人
 * 白敲一遍。
 */
function ServiceProductsField({ chosen, onChange }: { chosen: readonly string[]; onChange: (next: string[]) => void }) {
  const answer = useLoaded(listServiceProducts);
  const [typed, setTyped] = useState('');
  const options = answer?.kind === 'outcome' ? serviceProductObjectOptions(answer.body) : null;
  const choices = serviceProductChoices(options ?? [], chosen);
  const id = field.basics('serviceProducts');
  const labelId = `${id}.label`;
  const add = () => {
    onChange(addTypedServiceProducts(chosen, typed));
    setTyped('');
  };
  const unreadable =
    answer === null || answer.kind === 'outcome'
      ? null
      : answer.kind === 'unconfigured'
        ? '暂时读不到服务产品册（接入渠道未配置），先按标识添加，几个标识用英文分号隔开；标识对不对由预览时检查。'
        : isOperatorRefusal(answer)
          ? `暂时读不到服务产品册（${accessRefusalTitle(answer.kind)}），先按标识添加，几个标识用英文分号隔开；标识对不对由预览时检查。`
          : '服务产品册读不到，先按标识添加，几个标识用英文分号隔开；标识对不对由预览时检查。';

  return (
    <FieldBox id={id} label="适用服务产品" labelsControl={false} labelId={labelId} className="md:col-span-2" hint={restProductsHint}>
      <div className="space-y-2" data-field={id} tabIndex={-1} role="group" aria-labelledby={labelId}>
        {choices.length > 0 ? (
          <div className="flex flex-wrap gap-x-4 gap-y-1.5">
            {choices.map((choice) => (
              <label key={choice.value} className="flex items-center gap-1.5 type-value text-idpxyz-text">
                <input type="checkbox" checked={choice.checked} onChange={(event) => onChange(toggleServiceProduct(chosen, choice.value, event.target.checked))} />
                <span className={choice.listed ? 'font-mono' : 'font-mono text-idpxyz-breadcrumb'}>{choice.label}</span>
              </label>
            ))}
          </div>
        ) : null}
        {answer === null ? <p className={hintClass}>正在读服务产品册…</p> : null}
        {options !== null && options.length === 0 ? <p className={hintClass}>还没有发布任何服务产品，这一格可以不选。</p> : null}
        {unreadable !== null ? (
          <>
            <div className="flex max-w-md gap-2">
              <Input
                value={typed}
                className="font-mono type-value"
                placeholder="服务产品标识"
                aria-label="按标识添加服务产品"
                onChange={(event) => setTyped(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault();
                    add();
                  }
                }}
              />
              <Button variant="outline" size="sm" onClick={add} disabled={typed.trim() === ''}>
                添加
              </Button>
            </div>
            <p className={hintClass}>{unreadable}</p>
          </>
        ) : null}
      </div>
    </FieldBox>
  );
}

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
            onChange={(direction) => update((draft) => ({ ...draft, basics: basicsWithDirection(draft.basics, direction, purposes) }))}
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
          {/* 只有售价卡能声明适用产品（ADR-0378 决定一）；换成别的方向时已选的随之清掉，见 basicsWithDirection。
              规格口的模板版本收不收这一格由规格口说，不收就只说一句，不摆一个提交不上去的选择。 */}
          {declaresServiceProducts(basics.direction) ? (
            specTakesServiceProducts(spec) ? (
              <ServiceProductsField chosen={basics.serviceProducts} onChange={(serviceProducts) => patch({ serviceProducts })} />
            ) : (
              <p className={`${hintClass} md:col-span-2`}>{serviceProductsNotTakenNote}</p>
            )
          ) : null}
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
        <summary className="cursor-pointer select-none type-body text-idpxyz-breadcrumb">更多设置</summary>
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
