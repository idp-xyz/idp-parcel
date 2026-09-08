import { useEffect, useState, type ReactNode } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import { listPriceCards, type PriceCardListResponseBody } from '../pricing/api';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { planReferenceOf } from './supplier-agreement-form';
import {
  declaresFx,
  emptyPricePolicyDraft,
  planBindingConversionOptions,
  priceDirectionOptions,
  pricePolicyFieldPaths,
  pricePolicyPayloadOf,
  showsTaxClassification,
  showsVolumetricFactor,
  taxDispositionOptions,
  withShellCopiedIntoPolicy,
  type ClosedSetOption,
  type PricePolicyDraft,
} from './price-policy-form';

/**
 * 价格规则版本的逐字段表单（票 admin-write-faces/14；ADR-0101 决定八本册选形：逐字段表单，口径作条件节）。
 *
 * **为什么是逐字段表单**：低频、运营配置员操作、正文十来格无子表——决定一那三条的直接读数；模板导入只针对上百格的
 * 矩阵型价卡。五步（预览摘要 → 存为待批准 → 批准 → 发布）由公共半边 PublicationDraftFlow 走，本组件只摆版本壳五格、
 * 政策正文七格与口径节，草稿 → 载荷在 price-policy-form.ts。
 *
 * **方案从价卡目录选**：跨上下文只传引用，选出来的是 `planId@planVersion` 引用串，表单不读方案内容。目录行上的方向、
 * 用途、范围显给人看，**不按方向过滤、不预选**。`planDirection` 与 `conversion` 是发布当时保全的邻接答复与声明
 * （ADR-0057）：操作者照目录上看到的方案方向如实填，表单不从选中的方案反推、不代填 NONE。
 *
 * **口径作条件节，显隐是呈现不是裁门**：税务分类只在含税 / 未税时显、体积系数只在销售方向时显；隐掉的格草稿里有值照送，
 * 组件把它与服务端答在那一格的问题如实显出来并给「清空」，不静默丢。汇率三格全空即不声明汇率口径，填了任一格整节送。
 * 口径随正文同一份载荷送出，不给「先发正文、回头补口径」的两步。
 *
 * **表单不算摘要、不收也不送批准人、不裁任何门**（伞票 07 硬句）：连「必填」都不在本地拦；没有一格编不进载荷类型（全是
 * 文本），所以不给 localProblems。读面在接入渠道未配置那堵墙前（403）或读不到时退回手填，表单不因此变死。
 */
export interface PricePolicyPublicationFormProps {
  /** 载体到达「发布」那一步时回调，政策页借它切到商业价格政策册并重读。 */
  onPublished?: () => void;
}

const fieldLabel = 'block text-[12px] text-idpxyz-textMuted mb-1';
const groupTitle = 'text-[13px] font-medium text-idpxyz-text';
const noteClass = 'text-[11px] text-idpxyz-textMuted';
const selectClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 text-[13px] ' +
  'text-idpxyz-text focus:outline-none focus:border-idpxyz-accent disabled:opacity-60';
const momentPlaceholder = 'RFC 3339 或 YYYY-MM-DD（只到天补成当天零点 UTC）';

export function PricePolicyPublicationForm({ onPublished }: PricePolicyPublicationFormProps) {
  const [draft, setDraft] = useState<PricePolicyDraft>(emptyPricePolicyDraft());
  const patch = (change: Partial<PricePolicyDraft>) => setDraft((current) => ({ ...current, ...change }));

  return (
    <PublicationDraftFlow
      kind="PRICE_RULE"
      title="发布价格政策版本"
      assemblePayload={() => pricePolicyPayloadOf(draft)}
      fieldPaths={pricePolicyFieldPaths}
      onPublished={onPublished}
    >
      {(form) => <PricePolicyFields draft={draft} patch={patch} setDraft={setDraft} form={form} />}
    </PublicationDraftFlow>
  );
}

function PricePolicyFields({
  draft,
  patch,
  setDraft,
  form,
}: {
  draft: PricePolicyDraft;
  patch: (change: Partial<PricePolicyDraft>) => void;
  setDraft: (update: (current: PricePolicyDraft) => PricePolicyDraft) => void;
  form: PublicationFormContext;
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

  return (
    <div className="flex flex-col gap-5">
      <section className="flex flex-col gap-3">
        <h3 className={groupTitle}>
          版本壳 <span className="font-mono text-idpxyz-textMuted">kind = PRICE_RULE</span>
        </h3>
        <p className={noteClass}>
          壳上的适用范围与区间是<strong>版本</strong>的（登记册逐列比对的项），与下面政策正文自己的范围与区间是两样；区间上界
          留空即持续有效。结果显示在政策页的<strong>商业价格政策册</strong>（册名 PRICE_POLICY，发布类别 PRICE_RULE——两条分类轴）。
        </p>
        <ProblemLines lines={problems['kind']} />
        <div className="grid grid-cols-2 gap-3">
          {text('objectId', '价格规则对象标识 *', 'objectId')}
          {text('version', '版本号 *', 'version')}
          {text('scope', '版本适用范围 *', 'scope')}
          <div />
          {text('effectiveStartsAt', '版本生效起点 *', 'effectiveStartsAt', momentPlaceholder)}
          {text('effectiveEndsAt', '版本生效止点（可空）', 'effectiveEndsAt', '留空即持续有效')}
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <h3 className={groupTitle}>政策正文（商业价格政策，0010）</h3>
          <Button variant="outline" disabled={locked} onClick={() => setDraft(withShellCopiedIntoPolicy)}>
            范围与区间从版本壳带入
          </Button>
        </div>
        <p className={noteClass}>
          方向 × 方案绑定：政策授权哪个价格方向、绑定 parcel-pricing 的哪份可执行定价方案。方案从价卡目录<strong>选</strong>，
          送上去的只是引用串（planId@planVersion），表单不读方案内容。<strong>方案方向与转换如实填</strong>（ADR-0057）：方案方向是
          发布当时 parcel-pricing 的答复——照目录行上显示的方向填，表单不从选中的方案反推；转换只在方向不一致时有意义，同向须填
          NONE、跨向只许 SELL 政策引用一次已冻结的 BUY 评价。没填或填错的组合由服务端答（AT-PC-033），预览时以成因一句回来。
        </p>
        <div className="grid grid-cols-2 gap-3">
          {closedSet('pricePolicy.direction', '政策方向 *', 'direction', priceDirectionOptions)}
          <PriceCardPicker
            label="定价方案（价卡目录）*"
            path="pricePolicy.pricingPlan"
            problems={problems}
            value={draft.pricingPlan}
            locked={locked}
            onChange={(pricingPlan) => patch({ pricingPlan })}
          />
          {closedSet('pricePolicy.planDirection', '方案方向（parcel-pricing 发布当时的答复）*', 'planDirection', priceDirectionOptions)}
          {closedSet('pricePolicy.conversion', '绑定转换 *', 'conversion', planBindingConversionOptions)}
          {text('pricePolicy.scope', '政策适用范围 *', 'policyScope')}
          <div />
          {text('pricePolicy.effectiveStartsAt', '政策生效起点 *', 'policyEffectiveStartsAt', momentPlaceholder)}
          {text('pricePolicy.effectiveEndsAt', '政策生效止点（可空）', 'policyEffectiveEndsAt', '留空即无上界')}
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <h3 className={groupTitle}>计价口径（0022，随正文同一份载荷登记）</h3>
        <p className={noteClass}>
          口径随正文<strong>同笔</strong>登记，不给「先发正文、回头补口径」的两步。税务口径三选一；税务分类只在含税 / 未税时
          显、体积系数只在销售方向时显——<strong>显隐是呈现，不是裁门</strong>：显了没填、隐了却传了，都由服务端按库上 CHECK
          同形的构造门答在那一格。汇率口径可缺（不涉及外币）：三格全空即不声明；填了任一格整节送，缺的由服务端点名。
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
            hiddenNote="税务不适用时不显此格"
            placeholder="含税 / 未税时必填（税务主数据上的分类引用）"
            onChange={(taxClassification) => patch({ taxClassification })}
          />
          <ConditionalField
            shown={showsVolumetricFactor(draft)}
            label="体积系数引用"
            path="pricePolicy.caliber.volumetricFactor"
            problems={problems}
            value={draft.volumetricFactor}
            locked={locked}
            hiddenNote="采购与法人间方向不显此格（系数在承运商价卡上）"
            placeholder="销售方向必填（版本化声明的体积系数引用）"
            onChange={(volumetricFactor) => patch({ volumetricFactor })}
          />
        </div>

        <div>
          <span className={fieldLabel}>
            汇率口径（可缺；{declaresFx(draft) ? '已填，整节随载荷送出' : '三格全空，不声明汇率口径'}）
            <span className="font-mono text-[10px] ml-1">pricePolicy.caliber.fx</span>
          </span>
          <div className="grid grid-cols-3 gap-3">
            {text('pricePolicy.caliber.fx.quoteType', '牌价类型引用', 'fxQuoteType', '租户与其银行约定的牌价类型')}
            {text('pricePolicy.caliber.fx.asOfSemantics', '取值时点语义引用', 'fxAsOfSemantics')}
            {text('pricePolicy.caliber.fx.asOfPolicyVersion', '时点政策版本', 'fxAsOfPolicyVersion')}
          </div>
          <ProblemLines lines={problems['pricePolicy.caliber.fx']} />
        </div>
      </section>
    </div>
  );
}

// 一格：标签、控件、服务端点名到这条路径的问题（构造门原话，可多条）。
function Field({
  label,
  path,
  problems,
  children,
}: {
  label: string;
  path: string;
  problems: Record<string, string[]>;
  children: ReactNode;
}) {
  return (
    <label className="block">
      <span className={fieldLabel}>
        {label} <span className="font-mono text-[10px]">{path}</span>
      </span>
      {children}
      <ProblemLines lines={problems[path]} />
    </label>
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
      <span className={fieldLabel}>
        {label} <span className="font-mono text-[10px]">{path}</span>
      </span>
      {value.trim() === '' ? (
        <span className={`${noteClass} block mt-1.5`}>{hiddenNote}。</span>
      ) : (
        <div className="flex items-center gap-2 mt-1">
          <span className={noteClass}>
            {hiddenNote}，但草稿里仍有值 <span className="font-mono">{value}</span>，会随载荷照送，由服务端答。
          </span>
          <Button variant="outline" disabled={locked} onClick={() => onChange('')}>
            清空
          </Button>
        </div>
      )}
      <ProblemLines lines={problems[path]} />
    </div>
  );
}

// 服务端点名的那一格的问题，逐条挂在格下；原话原样显示，不改写。
function ProblemLines({ lines }: { lines?: string[] }) {
  if (!lines || lines.length === 0) return null;
  return (
    <ul className="text-[11px] text-idpxyz-danger list-disc ml-4 mt-1">
      {lines.map((line) => (
        <li key={line}>{line}</li>
      ))}
    </ul>
  );
}

/**
 * 从价卡目录选一份方案版本。目录答了业务答案就给选单——每行显方案引用、方向、用途、范围，**不按方向过滤**（表单不裁，
 * 方案方向由操作者照这里看到的如实填进「方案方向」那格，表单不替他填）；目录在未配置那堵墙前或读不到时退回手填并说明
 * 原因。选出来的只是引用串，在不在册、方向对不对仍由服务端判。手填时若当前值不在候选里，选单照样保留它作一项。
 * 形状与 SupplierAgreementPublicationForm 的 ReferencePicker 同，那一份是它文件的私有件、这里不跨文件借。
 */
function PriceCardPicker({
  label,
  path,
  problems,
  value,
  locked,
  onChange,
}: {
  label: string;
  path: string;
  problems: Record<string, string[]>;
  value: string;
  locked: boolean;
  onChange: (value: string) => void;
}) {
  const [answer, setAnswer] = useState<ApiResult<PriceCardListResponseBody> | null>(null);
  useEffect(() => {
    let cancelled = false;
    void listPriceCards().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  if (answer?.kind === 'outcome') {
    const options = answer.body.cards.map((card) => ({
      value: planReferenceOf(card),
      label: `${planReferenceOf(card)} · ${card.direction} · ${card.purpose} · ${card.scope}`,
    }));
    const known = options.some((option) => option.value === value);
    return (
      <Field label={label} path={path} problems={problems}>
        <select className={selectClass} value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
          <option value="">未选</option>
          {!known && value !== '' ? <option value={value}>{value}（手填，不在目录上）</option> : null}
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <span className={`${noteClass} block mt-1`}>
          {options.length === 0
            ? '价卡目录为空；先登记价卡，再发布价格政策。'
            : '目录行上的方向是 parcel-pricing 的答复，照它填「方案方向」；表单不从选中的方案反推。'}
        </span>
      </Field>
    );
  }

  return (
    <Field label={label} path={path} problems={problems}>
      <Input
        value={value}
        disabled={locked}
        className="font-mono text-[13px]"
        placeholder="planId@planVersion（目录不可用时手填）"
        onChange={(event) => onChange(event.target.value)}
      />
      <span className={`${noteClass} block mt-1`}>
        {answer === null
          ? '正在读价卡目录…'
          : answer.kind === 'unconfigured'
            ? '价卡目录读口在接入渠道未配置那堵墙前（403），先手填；引用在不在册由服务端发布时判。'
            : '价卡目录读不到，先手填；引用在不在册由服务端发布时判。'}
      </span>
    </Field>
  );
}
