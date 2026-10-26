import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { currentDisplayTimeZone } from '../moment';
import { listPriceCards, type PriceCardListResponseBody } from '../pricing/api';
import type { NewVersionStart } from './new-version';
import { listCommercialPolicies } from './api';
import { ExistingVersionsNote } from './ExistingVersionsNote';
import { policyKnownVersions } from './known-versions';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { Field, Problems, ReferencePicker, fieldLabel, selectClass, useLoaded } from './PublicationFormFields';
import { WallTimeField } from './party-registration-fields';
import { planReferenceOf } from './publication-form-shared';
import {
  declaresFx,
  emptyPricePolicyDraft,
  planBindingConversionOptions,
  priceDirectionOptions,
  pricePolicyFieldPaths,
  pricePolicyLocalProblems,
  pricePolicyPayloadOf,
  showsTaxClassification,
  taxDispositionOptions,
  withShellCopiedIntoPolicy,
  type ClosedSetOption,
  type PricePolicyDraft,
} from './price-policy-form';

const loadKnownVersions = () => listCommercialPolicies('PRICE_POLICY');

/**
 * 价格规则版本的逐字段表单（票 admin-write-faces/14；ADR-0101 决定八本册选形：逐字段表单，口径作条件节）。
 *
 * **为什么是逐字段表单**：低频、运营配置员操作、正文十来格无子表——决定一判据（登记频次 × 操作者角色 × 载荷结构）
 * 的直接读数；模板导入只针对上百格的矩阵型价卡。五步（预览摘要 → 存为待批准 → 批准 → 发布）由公共半边 PublicationDraftFlow 走，本组件只摆版本壳五格、
 * 政策正文七格与口径节，草稿 → 载荷在 price-policy-form.ts。
 *
 * **方案从价卡目录选**：跨上下文只传引用，选出来的是 `planId@planVersion` 引用串，表单不读方案内容。目录行上的方向、
 * 用途、范围显给人看，**不按方向过滤、不预选**。`planDirection` 与 `conversion` 是发布当时保全的邻接答复与声明
 * （ADR-0057）：操作者照目录上看到的方案方向如实填，表单不从选中的方案反推、不代填 NONE。
 *
 * **口径作条件节，显隐是呈现不是裁门**：税务分类只在含税 / 未税时显；隐掉的格草稿里有值照送，
 * 组件把它与服务端答在那一格的问题如实显出来并给「清空」，不静默丢。汇率三格全空即不声明汇率口径，填了任一格整节送。
 * 口径随正文同一份载荷送出，不给「先发正文、回头补口径」的两步。
 *
 * **表单不算摘要、不收也不送批准人、不裁任何门**（伞票 07 硬句）：连「必填」都不在本地拦；本地只拦换不成时刻的时刻格
 * （pricePolicyLocalProblems），那是组不出载荷，不是校验。读面在接入渠道未配置那堵墙前（403）或读不到时退回手填，表单
 * 不因此变死。
 */
export interface PricePolicyPublicationFormProps {
  /** 载体到达「发布」那一步时回调，政策页借它切到商业价格政策册并重读。 */
  onPublished?: () => void;
  /** 基于目录里某一版预填（票 party-commercial-uiux/14）；换一次预填由页面换 key 重建本表单。 */
  start?: NewVersionStart<PricePolicyDraft>;
}

const groupTitle = 'text-[13px] font-medium text-idpxyz-text';
const noteClass = 'text-[11px] text-idpxyz-textMuted';

export function PricePolicyPublicationForm({ onPublished, start }: PricePolicyPublicationFormProps) {
  const [draft, setDraft] = useState<PricePolicyDraft>(start?.draft ?? emptyPricePolicyDraft());
  const patch = (change: Partial<PricePolicyDraft>) => setDraft((current) => ({ ...current, ...change }));
  // 时刻按操作者的显示时区填、送出 UTC（票 party-commercial-uiux/10），与登记表单同一条换算。
  const timeZone = currentDisplayTimeZone();
  // 这个对象已有哪些版本（票 party-commercial-uiux/15）：读不到就不提示，不报错。
  const knownAnswer = useLoaded(loadKnownVersions);
  const known = knownAnswer?.kind === 'outcome' ? policyKnownVersions(knownAnswer.body) : null;

  return (
    <PublicationDraftFlow
      kind="PRICE_RULE"
      title="发布价格政策版本"
      assemblePayload={() => pricePolicyPayloadOf(draft, timeZone)}
      localProblems={pricePolicyLocalProblems(draft, timeZone)}
      fieldPaths={pricePolicyFieldPaths}
      onPublished={onPublished}
      prefilledFrom={start?.from}
      prefilledCaution={start?.caution}
      versionNote={
        <ExistingVersionsNote
          known={known}
          objectId={draft.objectId}
          version={draft.version}
          timeZone={timeZone}
        />
      }
    >
      {(form) => <PricePolicyFields draft={draft} patch={patch} setDraft={setDraft} form={form} timeZone={timeZone} />}
    </PublicationDraftFlow>
  );
}

function PricePolicyFields({
  draft,
  patch,
  setDraft,
  form,
  timeZone,
}: {
  draft: PricePolicyDraft;
  patch: (change: Partial<PricePolicyDraft>) => void;
  setDraft: (update: (current: PricePolicyDraft) => PricePolicyDraft) => void;
  form: PublicationFormContext;
  timeZone: string;
}) {
  const { problems, locked } = form;
  const text = (path: string, label: string, key: keyof PricePolicyDraft, placeholder?: string) => (
    <Field label={label} path={path} problems={problems}>
      <Input
        value={draft[key]}
        disabled={locked}
        className="font-mono text-[13px]"
        placeholder={placeholder}
        onChange={(event) => patch({ [key]: event.target.value } as Partial<PricePolicyDraft>)}
      />
    </Field>
  );
  const closedSet = (path: string, label: string, key: keyof PricePolicyDraft, options: readonly ClosedSetOption[]) => (
    <Field label={label} path={path} problems={problems}>
      <select
        className={selectClass}
        value={draft[key]}
        disabled={locked}
        onChange={(event) => patch({ [key]: event.target.value } as Partial<PricePolicyDraft>)}
      >
        <option value="">未选</option>
        {!options.some((option) => option.value === draft[key]) && draft[key] !== '' ? (
          <option value={draft[key]}>{draft[key]}（集外，原样示出）</option>
        ) : null}
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </Field>
  );
  const moment = (path: string, label: string, key: keyof PricePolicyDraft, note?: string) => (
    <WallTimeField
      label={label}
      path={path}
      problems={problems}
      value={draft[key]}
      locked={locked}
      timeZone={timeZone}
      note={note}
      onChange={(value) => patch({ [key]: value } as Partial<PricePolicyDraft>)}
    />
  );

  return (
    <div className="flex flex-col gap-5">
      <section className="flex flex-col gap-3">
        <h3 className={groupTitle}>版本信息</h3>
        {/* 版本上的范围与区间是登记册逐列比对的项，与正文自己的范围与区间是两样。册名 PRICE_POLICY 与发布类别 PRICE_RULE
            是两条分类轴，上屏只说册的中文名。 */}
        <p className={noteClass}>
          这里的适用范围与有效区间属于<strong>版本</strong>，和下面政策正文自己的范围与区间分开填。发布后在政策页的
          <strong>商业价格政策</strong>册查看。
        </p>
        <Problems lines={problems['kind']} />
        <div className="grid grid-cols-2 gap-3">
          {text('objectId', '价格规则对象标识 *', 'objectId')}
          {text('version', '版本号 *', 'version')}
          {text('scope', '版本适用范围 *', 'scope')}
          <div />
          {moment('effectiveStartsAt', '版本生效起点 *', 'effectiveStartsAt')}
          {moment('effectiveEndsAt', '版本生效止点', 'effectiveEndsAt', '留空即持续有效。')}
        </div>
        {form.versionNote}
      </section>

      <section className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <h3 className={groupTitle}>政策正文（这一版的条款内容）</h3>
          <Button variant="outline" disabled={locked} onClick={() => setDraft(withShellCopiedIntoPolicy)}>
            范围与区间从版本信息带入
          </Button>
        </div>
        {/* 方案方向是发布当时价卡上下文的答复，照目录行填、表单不从选中的方案反推（ADR-0057）；方向 × 方案方向 × 转换的
            组合由服务端判（AT-PC-033），预览时以成因一句回来。 */}
        <p className={noteClass}>
          政策方向说明这条政策定的是买价、卖价还是内部价；定价方案从价卡目录<strong>选</strong>。
          <strong>方案方向照目录行上显示的方向填</strong>
          ，表单不会替你推断。绑定转换：两个方向相同时选「不转换」；方向不同时，只有卖价政策可以引用一次已冻结的采购评价。组合是否成立由服务端在预览时检查，并说明原因。
        </p>
        <div className="grid grid-cols-2 gap-3">
          {closedSet('pricePolicy.direction', '政策方向 *', 'direction', priceDirectionOptions)}
          {/* 从价卡目录选一份方案版本：每行显方案引用、方向、用途、范围，**不按方向过滤**（表单不裁，方案方向由操作者照这里
              看到的如实填进「方案方向」那格，表单不替他填）；选出来的只是引用串，在不在册、方向对不对仍由服务端判。 */}
          <ReferencePicker
            label="定价方案（价卡目录）*"
            path="pricePolicy.pricingPlan"
            problems={problems}
            value={draft.pricingPlan}
            locked={locked}
            onChange={(pricingPlan) => patch({ pricingPlan })}
            load={listPriceCards}
            optionsOf={(body: PriceCardListResponseBody) =>
              body.cards.map((card) => ({
                value: planReferenceOf(card),
                label: `${planReferenceOf(card)} · ${card.direction} · ${card.purpose} · ${card.scope}`,
              }))
            }
            emptyNote="价卡目录为空；先登记价卡，再发布价格政策。"
            optionsNote="照所选目录行上的方向填「方案方向」；表单不会替你推断。"
            unknownNote="手填，不在目录上"
            manualPlaceholder="方案标识@方案版本（目录不可用时手填）"
            readFace="价卡目录"
          />
          {closedSet('pricePolicy.planDirection', '方案方向（照价卡目录上的方向）*', 'planDirection', priceDirectionOptions)}
          {closedSet('pricePolicy.conversion', '绑定转换 *', 'conversion', planBindingConversionOptions)}
          {text('pricePolicy.scope', '政策适用范围 *', 'policyScope')}
          <div />
          {moment('pricePolicy.effectiveStartsAt', '政策生效起点 *', 'policyEffectiveStartsAt')}
          {moment('pricePolicy.effectiveEndsAt', '政策生效止点', 'policyEffectiveEndsAt', '留空即持续有效。')}
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <h3 className={groupTitle}>计价口径（随正文一起登记）</h3>
        {/* 显隐是呈现不是裁门：显了没填、隐了却传了，都由服务端按库上 CHECK 同形的构造门答在那一格。体积系数与金额取整
            是价卡内容（ADR-0260），本表单不给这一格。 */}
        <p className={noteClass}>
          口径和正文<strong>一起</strong>
          登记，不能先发正文、事后补口径。税务口径三选一；选含税或未税时要填税务分类。不涉及外币时汇率口径可以不填：三格全空即不声明，填了任一格就要三格填齐。体积系数与金额取整在价卡上登记，不在价格政策上。
        </p>
        <div className="grid grid-cols-2 gap-3">
          {closedSet('pricePolicy.caliber.taxDisposition', '税务口径 *', 'taxDisposition', taxDispositionOptions)}
          <ConditionalField
            shown={showsTaxClassification(draft)}
            label="税务分类引用"
            path="pricePolicy.caliber.taxClassification"
            problems={problems}
            value={draft.taxClassification}
            locked={locked}
            hiddenNote="税务不适用时不需要这一格"
            placeholder="含税 / 未税时必填（税务主数据上的分类引用）"
            onChange={(taxClassification) => patch({ taxClassification })}
          />
        </div>

        <div>
          <span className={fieldLabel}>
            汇率口径（可不填；{declaresFx(draft) ? '已填写，三格需填齐' : '三格全空，不声明汇率口径'}）
          </span>
          <div className="grid grid-cols-3 gap-3">
            {text('pricePolicy.caliber.fx.quoteType', '牌价类型引用', 'fxQuoteType', '租户与其银行约定的牌价类型')}
            {text('pricePolicy.caliber.fx.asOfSemantics', '取值时点语义引用', 'fxAsOfSemantics')}
            {text('pricePolicy.caliber.fx.asOfPolicyVersion', '时点政策版本', 'fxAsOfPolicyVersion')}
          </div>
          <Problems lines={problems['pricePolicy.caliber.fx']} />
        </div>
      </section>
    </div>
  );
}

/**
 * 条件格。显时是普通一格；隐时若草稿里仍有值，把值、「随载荷照送」的事实与服务端答在这一格的问题如实显出来并给「清空」
 * ——隐掉不等于丢掉，丢掉是操作者自己按的。隐且空时只留一句为什么不显。
 */
function ConditionalField({
  shown,
  label,
  path,
  problems,
  value,
  locked,
  hiddenNote,
  placeholder,
  onChange,
}: {
  shown: boolean;
  label: string;
  path: string;
  problems: Record<string, string[]>;
  value: string;
  locked: boolean;
  hiddenNote: string;
  placeholder: string;
  onChange: (value: string) => void;
}) {
  if (shown) {
    return (
      <Field label={label} path={path} problems={problems}>
        <Input
          value={value}
          disabled={locked}
          className="font-mono text-[13px]"
          placeholder={placeholder}
          onChange={(event) => onChange(event.target.value)}
        />
      </Field>
    );
  }
  return (
    <div className="block">
      <span className={fieldLabel}>{label}</span>
      {value.trim() === '' ? (
        <span className={`${noteClass} block mt-1.5`}>{hiddenNote}。</span>
      ) : (
        <div className="flex items-center gap-2 mt-1">
          <span className={noteClass}>
            {hiddenNote}，但这里仍填着 <span className="font-mono">{value}</span>，会照样提交并由服务端检查；不需要就清空。
          </span>
          <Button variant="outline" disabled={locked} onClick={() => onChange('')}>
            清空
          </Button>
        </div>
      )}
      <Problems lines={problems[path]} />
    </div>
  );
}
