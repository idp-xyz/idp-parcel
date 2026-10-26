import { useState, type ReactNode } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { accessRefusalTitle } from '../../auth/access-refusal';
import type { ApiResult } from '../catalogue-api';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { fetchPublicationVocabulary, type PublicationVocabularyResponseBody } from './publication-draft-api';
import { Field, Problems, RowFrame, fieldLabel, selectClass, useLoaded } from './PublicationFormFields';
import {
  acceptanceRulePackageFieldPaths,
  emptyAcceptanceRulePackageDraft,
  emptyAmendmentRuleDraft,
  emptyAsOfPolicyDraft,
  emptyAssembledRuleDraft,
  emptyFinalRuleDraft,
  payloadOf,
  sectionDeclared,
  setOptionsOf,
  vocabularyStateOf,
  type AcceptanceRulePackageDraft,
  type ClosedDraft,
  type DeclarationSection,
  type RulePackageBodyDraft,
  type SetOptionsState,
  type VocabularySetName,
  type VocabularyState,
} from './acceptance-rule-package-form';

/**
 * 接单规则包版本的分节发布表单（票 admin-write-faces/12；ADR-0101 决定八自裁为分节的逐字段表单，不走模板导入）。
 * 五步「表单 → 预览摘要 → 存为待批准 → 批准 → 发布」由 PublicationDraftFlow 走，本组件只摆本册的几节并把草稿组成
 * 载荷（纯函数在 acceptance-rule-package-form.ts）。
 *
 * **一份表单、分节：正文一节 + 每条声明通道各一节。** 一版规则包的发布是一次提交，正文与全部声明在同一份载荷、同一
 * 个摘要下（ADR-0042/0058 归属纪律与「声明只能随发布登记」）。**某一节整节留空 = 该通道未声明**：载荷里不带那一节，
 * 消费方照旧译`未配置`；表单不给任何一节默认值、不把「留空」写成「无」。每节标题旁写着它此刻是留空还是会随发布登记，
 * 那是从草稿算出来的事实，不是判断。
 *
 * **表单不算摘要、不裁任何门、不代判**（伞票 07 硬句）：没选的码、空的行、同一判断两条时点锚、空组、只有有效期没有
 * 终局行、未封闭零格都照样送上去，答回来的是构造门的拒绝（逐格问题落在格上，跨格的判在预览上答`未受理`带成因）。
 * **各节封闭集不内置枚举**：码从服务端词表读口一口取回（票 20），中文用本页词表；读口在未配置那堵墙前时下拉显占位，
 * 不拿任何内置码顶替，也不预选任何一项。`manualReview` 那一格答的是「要不要人工复核」，不是「谁有权」（票 12 硬句）。
 */
export interface AcceptanceRulePackagePublicationFormProps {
  /** 载体到达「发布」那一步时回调，页面借它刷同页目录读面（各节声明在册上各自的列里）。 */
  onPublished?: () => void;
}

const sectionTitle = 'text-[13px] font-medium text-idpxyz-text';
const hint = 'text-[11px] text-idpxyz-textMuted';
const root = 'acceptanceRulePackage';
// 本册 payloadOf 把时刻原样送出、不替只到天的日期补零点，服务端按完整时刻解析，只填日期会被拒——所以占位要求完整时刻，
// 不用共享层那句「只到天补成当天零点」。
const momentPlaceholder = '请填完整时刻，例如 2026-10-01T00:00:00+08:00';
const openEndPlaceholder = '留空即持续有效；要填请填完整时刻';

type Update = (mutate: (current: AcceptanceRulePackageDraft) => AcceptanceRulePackageDraft) => void;

function replaceAt<Row>(rows: Row[], index: number, change: Partial<Row>): Row[] {
  return rows.map((row, at) => (at === index ? { ...row, ...change } : row));
}

function removeAt<Row>(rows: Row[], index: number): Row[] {
  return rows.filter((_, at) => at !== index);
}

export function AcceptanceRulePackagePublicationForm({ onPublished }: AcceptanceRulePackagePublicationFormProps) {
  const [draft, setDraft] = useState<AcceptanceRulePackageDraft>(emptyAcceptanceRulePackageDraft());
  const vocabulary = useVocabulary();
  // 一律函数式更新：几节的行各自 onChange，取闭包里的旧草稿会让紧接的两次改动丢一次。
  const update: Update = (mutate) => setDraft((current) => mutate(current));

  return (
    <PublicationDraftFlow
      kind="ACCEPTANCE_RULE_PACKAGE"
      title="发布规则包版本"
      assemblePayload={() => payloadOf(draft)}
      fieldPaths={acceptanceRulePackageFieldPaths(draft)}
      onPublished={onPublished}
    >
      {(form) => (
        <div className="flex flex-col gap-5">
          <VocabularyNotice vocabulary={vocabulary} />
          <ShellFields draft={draft} update={update} form={form} />
          <RulePackageBodyFields draft={draft} update={update} form={form} vocabulary={vocabulary} />
          <AsOfPolicyFields draft={draft} update={update} form={form} vocabulary={vocabulary} />
          <AcceptanceContentFields draft={draft} update={update} form={form} vocabulary={vocabulary} />
          <IntakeQualificationFields draft={draft} update={update} form={form} vocabulary={vocabulary} />
          <FinalRuleFields draft={draft} update={update} form={form} vocabulary={vocabulary} />
          <SourceDataAmendmentFields draft={draft} update={update} form={form} vocabulary={vocabulary} />
        </div>
      )}
    </PublicationDraftFlow>
  );
}

interface SectionProps {
  draft: AcceptanceRulePackageDraft;
  update: Update;
  form: PublicationFormContext;
  vocabulary: VocabularyState;
}

// ——词表：一口读回本册十个封闭集（票 20）。读不到时只在表单顶上说一次为什么，各下拉自己显短占位、不可选、不内置码回退
// ——那堵墙前发布各口也答 403，这张表单本来就提交不了。

// 稳定的函数引用：useLoaded 以它为依赖，写成内联箭头会每次渲染重取。
function loadRulePackageVocabulary(): Promise<ApiResult<PublicationVocabularyResponseBody>> {
  return fetchPublicationVocabulary('ACCEPTANCE_RULE_PACKAGE');
}

function useVocabulary(): VocabularyState {
  return vocabularyStateOf(useLoaded(loadRulePackageVocabulary));
}

function VocabularyNotice({ vocabulary }: { vocabulary: VocabularyState }) {
  if (vocabulary.kind === 'listed') return null;
  return (
    <p className={hint}>
      {vocabulary.kind === 'loading'
        ? '正在读取各节的可选项；读到之前下拉暂时选不了。'
        : vocabulary.kind === 'unconfigured'
          ? '暂时读不到可选项：接入渠道还没有配置。配置好之前这张表单也提交不了。'
          : vocabulary.kind === 'unavailable'
            ? '暂时读不到可选项，请稍后重新打开本页签再试。'
            : `暂时读不到可选项（${accessRefusalTitle(vocabulary.kind)}）。`}
    </p>
  );
}

const setPlaceholder: Record<Exclude<SetOptionsState['kind'], 'options'>, string> = {
  loading: '正在读取可选项…',
  unconfigured: '暂时读不到可选项（接入渠道未配置）',
  loginRequired: `暂时读不到可选项（${accessRefusalTitle('loginRequired')}）`,
  notGranted: `暂时读不到可选项（${accessRefusalTitle('notGranted')}）`,
  unavailable: '暂时读不到可选项',
};

/** 一个封闭集的下拉：选项 = 服务端码 × 本页中文；首项是空的「未选」，不预选任何一格。 */
function CodeSelect({
  value,
  set,
  vocabulary,
  locked,
  onChange,
}: {
  value: string;
  set: VocabularySetName;
  vocabulary: VocabularyState;
  locked: boolean;
  onChange: (code: string) => void;
}) {
  const options = setOptionsOf(vocabulary, set);
  if (options.kind !== 'options') {
    return (
      <select className={selectClass} value="" disabled>
        <option value="">{setPlaceholder[options.kind]}</option>
      </select>
    );
  }
  return (
    <select className={selectClass} value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
      <option value="">未选</option>
      {options.options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  );
}

/**
 * 一个封闭集的多选：每个码一格勾选，选中的按服务端顺序成列表送上去。没有一格默认勾上；词表读不到时只显占位，
 * 没有任何码可勾。
 */
function CodeChecklist({
  selected,
  set,
  vocabulary,
  locked,
  onChange,
}: {
  selected: string[];
  set: VocabularySetName;
  vocabulary: VocabularyState;
  locked: boolean;
  onChange: (codes: string[]) => void;
}) {
  const options = setOptionsOf(vocabulary, set);
  if (options.kind !== 'options') {
    return <span className={hint}>{setPlaceholder[options.kind]}</span>;
  }
  const toggle = (code: string, checked: boolean) => {
    const next = new Set(selected);
    if (checked) next.add(code);
    else next.delete(code);
    onChange(options.options.map((option) => option.value).filter((candidate) => next.has(candidate)));
  };
  return (
    <div className="flex flex-wrap gap-x-4 gap-y-1">
      {options.options.map((option) => (
        <label key={option.value} className="inline-flex items-center gap-1 text-[13px] text-idpxyz-text">
          <input
            type="checkbox"
            checked={selected.includes(option.value)}
            disabled={locked}
            onChange={(event) => toggle(option.value, event.target.checked)}
          />
          {option.label}
        </label>
      ))}
    </div>
  );
}

// ——版本壳：身份三元（对象标识 / 版本号 / 适用范围）与有效区间。租户不在这里：它从操作者信封来。

function ShellFields({ draft, update, form }: Omit<SectionProps, 'vocabulary'>) {
  const patch = (change: Partial<AcceptanceRulePackageDraft>) => update((current) => ({ ...current, ...change }));
  return (
    <section className="flex flex-col gap-2">
      <h3 className={sectionTitle}>版本信息</h3>
      <div className="grid grid-cols-2 gap-3">
        <Field label="规则包对象标识 *" path="objectId" problems={form.problems}>
          <Input
            value={draft.objectId}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            onChange={(event) => patch({ objectId: event.target.value })}
          />
        </Field>
        <Field label="版本号 *" path="version" problems={form.problems}>
          <Input
            value={draft.version}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            onChange={(event) => patch({ version: event.target.value })}
          />
        </Field>
        <Field label="适用范围 *" path="scope" problems={form.problems}>
          <Input
            value={draft.scope}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="商业范围引用"
            onChange={(event) => patch({ scope: event.target.value })}
          />
        </Field>
        <div />
        <Field label="生效起点 *" path="effectiveStartsAt" problems={form.problems}>
          <Input
            value={draft.effectiveStartsAt}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder={momentPlaceholder}
            onChange={(event) => patch({ effectiveStartsAt: event.target.value })}
          />
        </Field>
        <Field label="生效止点（可空）" path="effectiveEndsAt" problems={form.problems}>
          <Input
            value={draft.effectiveEndsAt}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder={openEndPlaceholder}
            onChange={(event) => patch({ effectiveEndsAt: event.target.value })}
          />
        </Field>
      </div>
      <p className={hint}>起点晚于现在时，发布稿照样可以保存和批准；发布时会提示还没到生效起点，到了起点再来发布即可，这不是失败。</p>
    </section>
  );
}

// ——正文（0014）：五维适用性 + 按分类归档的规则引用表。本节不可缺；零行由领域拒（空规则包等于无条件接受）。

function RulePackageBodyFields({ draft, update, form, vocabulary }: SectionProps) {
  const path = `${root}.rulePackageBody`;
  const patchBody = (change: Partial<RulePackageBodyDraft>) =>
    update((current) => ({ ...current, body: { ...current.body, ...change } }));
  return (
    <section className="flex flex-col gap-2">
      <h3 className={sectionTitle}>规则包正文（这一版的条款内容）：适用条件与规则引用表</h3>
      <Problems lines={form.problems[path]} />
      <div className="grid grid-cols-2 gap-3">
        <Field label="服务产品 *" path={`${path}.serviceProduct`} problems={form.problems}>
          <Input
            value={draft.body.serviceProduct}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="服务产品对象标识"
            onChange={(event) => patchBody({ serviceProduct: event.target.value })}
          />
        </Field>
        <Field label="客户合同 *" path={`${path}.contract`} problems={form.problems}>
          <Input
            value={draft.body.contract}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="客户合同对象标识"
            onChange={(event) => patchBody({ contract: event.target.value })}
          />
        </Field>
        <Field label="责任法人 *" path={`${path}.legalEntity`} problems={form.problems}>
          <Input
            value={draft.body.legalEntity}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="责任法人引用"
            onChange={(event) => patchBody({ legalEntity: event.target.value })}
          />
        </Field>
        <Field label="正文适用范围 *" path={`${path}.scope`} problems={form.problems}>
          <Input
            value={draft.body.scope}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="商业范围引用"
            onChange={(event) => patchBody({ scope: event.target.value })}
          />
        </Field>
        <Field label="正文生效起点 *" path={`${path}.effectiveStartsAt`} problems={form.problems}>
          <Input
            value={draft.body.effectiveStartsAt}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder={momentPlaceholder}
            onChange={(event) => patchBody({ effectiveStartsAt: event.target.value })}
          />
        </Field>
        <Field label="正文生效止点（可空）" path={`${path}.effectiveEndsAt`} problems={form.problems}>
          <Input
            value={draft.body.effectiveEndsAt}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder={openEndPlaceholder}
            onChange={(event) => patchBody({ effectiveEndsAt: event.target.value })}
          />
        </Field>
      </div>
      <RowsHeader
        title="规则引用表"
        locked={form.locked}
        onAdd={() => patchBody({ rules: [...draft.body.rules, emptyAssembledRuleDraft()] })}
      />
      <p className={hint}>
        每行选一个分类，再填一条规则引用；规则包只引用规则，规则本身由执行它的那一方维护。至少要有一行。
      </p>
      <div className="flex flex-col gap-2">
        {draft.body.rules.map((rule, index) => {
          const row = `${path}.rules[${index}]`;
          return (
            <RowFrame key={index} path={row} problems={form.problems} locked={form.locked} onRemove={() => patchBody({ rules: removeAt(draft.body.rules, index) })}>
              <Field label="分类 *" path={`${row}.category`} problems={form.problems}>
                <CodeSelect
                  value={rule.category}
                  set="category"
                  vocabulary={vocabulary}
                  locked={form.locked}
                  onChange={(category) => patchBody({ rules: replaceAt(draft.body.rules, index, { category }) })}
                />
              </Field>
              <Field label="规则引用 *" path={`${row}.reference`} problems={form.problems}>
                <Input
                  value={rule.reference}
                  readOnly={form.locked}
                  className="font-mono text-[12px]"
                  placeholder="如 RULE/ingress-identity"
                  onChange={(event) => patchBody({ rules: replaceAt(draft.body.rules, index, { reference: event.target.value }) })}
                />
              </Field>
            </RowFrame>
          );
        })}
      </div>
    </section>
  );
}

// ——时点锚（0005）：每项判断适用哪种时点语义、依据该政策的哪个版本。同一判断两条由服务端拒。

function AsOfPolicyFields({ draft, update, form, vocabulary }: SectionProps) {
  const path = `${root}.asOfPolicies`;
  const rows = draft.asOfPolicies;
  const setRows = (next: typeof rows) => update((current) => ({ ...current, asOfPolicies: next }));
  return (
    <DeclarationSectionFrame
      title="接受判断时点策略：判断类型、时点语义与时点政策版本"
      section="asOfPolicies"
      draft={draft}
      update={update}
      locked={form.locked}
      onAdd={() => setRows([...rows, emptyAsOfPolicyDraft()])}
    >
      <p className={hint}>
        接受判断时点选择策略：每类接受判断按哪种时点取依据。每类判断只能写一行；时点语义与时点政策版本照原词填，这里不填具体时刻。
      </p>
      {rows.map((row, index) => {
        const rowPath = `${path}[${index}]`;
        return (
          <RowFrame key={index} path={rowPath} problems={form.problems} locked={form.locked} onRemove={() => setRows(removeAt(rows, index))} columns="grid-cols-[1fr_1fr_1fr_auto]">
            <Field label="判断类型 *" path={`${rowPath}.judgment`} problems={form.problems}>
              <CodeSelect
                value={row.judgment}
                set="judgment"
                vocabulary={vocabulary}
                locked={form.locked}
                onChange={(judgment) => setRows(replaceAt(rows, index, { judgment }))}
              />
            </Field>
            <Field label="时点语义引用 *" path={`${rowPath}.semantics`} problems={form.problems}>
              <Input
                value={row.semantics}
                readOnly={form.locked}
                className="font-mono text-[12px]"
                placeholder="如 AT_SUBMISSION"
                onChange={(event) => setRows(replaceAt(rows, index, { semantics: event.target.value }))}
              />
            </Field>
            <Field label="时点政策版本 *" path={`${rowPath}.policyVersion`} problems={form.problems}>
              <Input
                value={row.policyVersion}
                readOnly={form.locked}
                className="font-mono text-[12px]"
                placeholder="如 asof-policy/v1"
                onChange={(event) => setRows(replaceAt(rows, index, { policyVersion: event.target.value }))}
              />
            </Field>
          </RowFrame>
        );
      })}
    </DeclarationSectionFrame>
  );
}

// ——接受内容：哪些下游校验组适用、要不要人工复核。空组由服务端拒；「要不要」不是「谁有权」。

function AcceptanceContentFields({ draft, update, form, vocabulary }: SectionProps) {
  const path = `${root}.acceptanceContent`;
  const content = draft.acceptanceContent;
  const patchContent = (change: Partial<typeof content>) =>
    update((current) => ({ ...current, acceptanceContent: { ...current.acceptanceContent, ...change } }));
  const groupPaths = content.applicableGroups.map((_, index) => `${path}.applicableGroups[${index}]`);
  return (
    <DeclarationSectionFrame title="接受内容声明：适用校验组与人工复核" section="acceptanceContent" draft={draft} update={update} locked={form.locked}>
      <Problems lines={form.problems[path]} />
      <p className={hint}>
        至少勾选一个校验组。人工复核说的是「这类委托要不要人工业务复核」，不是「谁有权复核」——后者在授权规则里设。
      </p>
      {/* 多选是一排各带 label 的勾选框，外层用 div：label 套 label 会让点标题变成点第一格。 */}
      <Field
        label="适用校验组"
        path={groupPaths[0] ?? `${path}.applicableGroups[0]`}
        alsoPaths={groupPaths.slice(1)}
        problems={form.problems}
        as="div"
      >
        <CodeChecklist
          selected={content.applicableGroups}
          set="applicableGroups"
          vocabulary={vocabulary}
          locked={form.locked}
          onChange={(applicableGroups) => patchContent({ applicableGroups })}
        />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="人工复核" path={`${path}.manualReview`} problems={form.problems}>
          <CodeSelect
            value={content.manualReview}
            set="manualReview"
            vocabulary={vocabulary}
            locked={form.locked}
            onChange={(manualReview) => patchContent({ manualReview })}
          />
        </Field>
      </div>
    </DeclarationSectionFrame>
  );
}

// ——收寄资格（0013）：允许的来源与硬资格清单。真没有硬资格也要声明来源允许——来源在、清单显式为空，整节在场。

function IntakeQualificationFields({ draft, update, form, vocabulary }: SectionProps) {
  const path = `${root}.intakeQualification`;
  const intake = draft.intakeQualification;
  const patchIntake = (change: Partial<typeof intake>) =>
    update((current) => ({ ...current, intakeQualification: { ...current.intakeQualification, ...change } }));
  const sourcePaths = intake.sources.map((_, index) => `${path}.sources[${index}]`);
  return (
    <DeclarationSectionFrame
      title="收寄资格声明：允许来源与硬资格清单"
      section="intakeQualification"
      draft={draft}
      update={update}
      locked={form.locked}
      addLabel="加一条硬资格"
      onAdd={() => patchIntake({ qualifications: [...intake.qualifications, ''] })}
    >
      <Problems lines={form.problems[path]} />
      <p className={hint}>
        至少勾选一个允许来源。硬资格清单可以为空；即使没有硬资格，也要勾出允许的来源。
      </p>
      <Field
        label="允许来源"
        path={sourcePaths[0] ?? `${path}.sources[0]`}
        alsoPaths={sourcePaths.slice(1)}
        problems={form.problems}
        as="div"
      >
        <CodeChecklist
          selected={intake.sources}
          set="sources"
          vocabulary={vocabulary}
          locked={form.locked}
          onChange={(sources) => patchIntake({ sources })}
        />
      </Field>
      {intake.qualifications.map((qualification, index) => {
        const rowPath = `${path}.qualifications[${index}]`;
        return (
          <RowFrame
            key={index}
            path={rowPath}
            problems={form.problems}
            locked={form.locked}
            columns="grid-cols-[1fr_auto]"
            onRemove={() => patchIntake({ qualifications: removeAt(intake.qualifications, index) })}
          >
            <Field label="硬资格引用 *" path={rowPath} problems={form.problems} silent>
              <Input
                value={qualification}
                readOnly={form.locked}
                className="font-mono text-[12px]"
                placeholder="如 INTAKE-QUAL/realname"
                onChange={(event) =>
                  patchIntake({ qualifications: intake.qualifications.map((item, at) => (at === index ? event.target.value : item)) })
                }
              />
            </Field>
          </RowFrame>
        );
      })}
    </DeclarationSectionFrame>
  );
}

// ——终局规则（0013）：责任结果 × 终局种类，可加行；面单有效期（ADR-0119）是同一通道上的一格，随终局规则一起声明。
// 只填有效期不填终局行由服务端整项拒；整格留空 = 未声明有效期（成功结果不失效），表单不给默认时长。

function FinalRuleFields({ draft, update, form, vocabulary }: SectionProps) {
  const path = `${root}.finalRules`;
  const validityPath = `${root}.finalRuleValidity`;
  const rows = draft.finalRules;
  const setRows = (next: typeof rows) => update((current) => ({ ...current, finalRules: next }));
  const validity = draft.finalRuleValidity;
  const patchValidity = (change: Partial<typeof validity>) =>
    update((current) => ({ ...current, finalRuleValidity: { ...current.finalRuleValidity, ...change } }));
  return (
    <DeclarationSectionFrame
      title="终局规则声明：责任结果与终局种类"
      section="finalRules"
      draft={draft}
      update={update}
      locked={form.locked}
      onAdd={() => setRows([...rows, emptyFinalRuleDraft()])}
    >
      <p className={hint}>
        责任结果从下拉里选；终局种类各产品不统一，照原词填。
      </p>
      {rows.map((row, index) => {
        const rowPath = `${path}[${index}]`;
        return (
          <RowFrame key={index} path={rowPath} problems={form.problems} locked={form.locked} onRemove={() => setRows(removeAt(rows, index))}>
            <Field label="责任结果 *" path={`${rowPath}.outcome`} problems={form.problems}>
              <CodeSelect
                value={row.outcome}
                set="outcome"
                vocabulary={vocabulary}
                locked={form.locked}
                onChange={(outcome) => setRows(replaceAt(rows, index, { outcome }))}
              />
            </Field>
            <Field label="终局种类 *" path={`${rowPath}.finalKind`} problems={form.problems}>
              <Input
                value={row.finalKind}
                readOnly={form.locked}
                className="font-mono text-[12px]"
                placeholder="如 FINAL/delivery"
                onChange={(event) => setRows(replaceAt(rows, index, { finalKind: event.target.value }))}
              />
            </Field>
          </RowFrame>
        );
      })}
      <div className="rounded border border-dashed border-idpxyz-border p-2 flex flex-col gap-2 mt-1">
        <div className="flex items-center justify-between">
          <span className={fieldLabel}>面单有效期（随终局规则一起声明；两格都留空即不设有效期，成功结果不失效）</span>
          <SectionState declared={sectionDeclared(draft, 'finalRuleValidity')} />
        </div>
        <Problems lines={form.problems[validityPath]} />
        <div className="grid grid-cols-2 gap-3">
          <Field label="起算时刻种类" path={`${validityPath}.anchor`} problems={form.problems}>
            <CodeSelect
              value={validity.anchor}
              set="anchor"
              vocabulary={vocabulary}
              locked={form.locked}
              onChange={(anchor) => patchValidity({ anchor })}
            />
          </Field>
          {/* 时长收 ISO-8601 的一个子集 P[nD][T[nH][nM][nS]]：只有天、时、分、秒，年 / 月 / 周不收。 */}
          <Field label="时长" path={`${validityPath}.duration`} problems={form.problems}>
            <Input
              value={validity.duration}
              readOnly={form.locked}
              className="font-mono text-[13px]"
              placeholder="如 P7D（7 天）或 P3DT12H（3 天 12 小时）；不收年、月、周"
              onChange={(event) => patchValidity({ duration: event.target.value })}
            />
          </Field>
        </div>
        <p className={hint}>
          时长没有默认值，不能为零。填了有效期就要有终局规则行。
        </p>
      </div>
    </DeclarationSectionFrame>
  );
}

// ——资料修订允许（ADR-0120）：closed 一格 + 资料组 × 阶段 × 意图 × 允许性的表。closed 是登记方自己说的一句话，表单不给默认。

function SourceDataAmendmentFields({ draft, update, form, vocabulary }: SectionProps) {
  const path = `${root}.sourceDataAmendment`;
  const amendment = draft.sourceDataAmendment;
  const patchAmendment = (change: Partial<typeof amendment>) =>
    update((current) => ({ ...current, sourceDataAmendment: { ...current.sourceDataAmendment, ...change } }));
  // 「缺格」是表里没写到的（资料组 × 阶段 × 意图）那一格；closed = false 读作转复核，true 读作不允许（ADR-0120）。
  const closedChoices: { value: Exclude<ClosedDraft, ''>; label: string }[] = [
    { value: 'false', label: '没写到的转复核' },
    { value: 'true', label: '没写到的一律不允许' },
  ];
  return (
    <DeclarationSectionFrame
      title="资料修订允许声明：资料组、阶段、意图与允许性"
      section="sourceDataAmendment"
      draft={draft}
      update={update}
      locked={form.locked}
      onAdd={() => patchAmendment({ rules: [...amendment.rules, emptyAmendmentRuleDraft()] })}
    >
      <Problems lines={form.problems[path]} />
      <p className={hint}>
        先选表里没写到的修订怎么算：转复核，或一律不允许，没有默认。选「一律不允许」时表可以留空，意思是这一版什么资料都不许改；选「转复核」时至少要写一行。每行的允许性只有「允许 / 不允许」两种。
      </p>
      <Field label="表里没写到的修订怎么算 *" path={`${path}.closed`} problems={form.problems} as="div">
        <div className="flex items-center gap-1">
          {closedChoices.map((choice) => (
            <Button
              key={choice.value}
              variant={amendment.closed === choice.value ? 'default' : 'outline'}
              disabled={form.locked}
              onClick={() => patchAmendment({ closed: choice.value })}
            >
              {choice.label}
            </Button>
          ))}
        </div>
      </Field>
      {amendment.rules.map((rule, index) => {
        const rowPath = `${path}.rules[${index}]`;
        return (
          <RowFrame
            key={index}
            path={rowPath}
            problems={form.problems}
            locked={form.locked}
            columns="grid-cols-[1fr_1fr_1fr_1fr_auto]"
            onRemove={() => patchAmendment({ rules: removeAt(amendment.rules, index) })}
          >
            <Field label="资料组 *" path={`${rowPath}.dataGroup`} problems={form.problems}>
              <Input
                value={rule.dataGroup}
                readOnly={form.locked}
                className="font-mono text-[12px]"
                placeholder="如 consignee.address"
                onChange={(event) => patchAmendment({ rules: replaceAt(amendment.rules, index, { dataGroup: event.target.value }) })}
              />
            </Field>
            <Field label="阶段 *" path={`${rowPath}.stage`} problems={form.problems}>
              <CodeSelect
                value={rule.stage}
                set="stage"
                vocabulary={vocabulary}
                locked={form.locked}
                onChange={(stage) => patchAmendment({ rules: replaceAt(amendment.rules, index, { stage }) })}
              />
            </Field>
            <Field label="意图 *" path={`${rowPath}.intent`} problems={form.problems}>
              <CodeSelect
                value={rule.intent}
                set="intent"
                vocabulary={vocabulary}
                locked={form.locked}
                onChange={(intent) => patchAmendment({ rules: replaceAt(amendment.rules, index, { intent }) })}
              />
            </Field>
            <Field label="允许性 *" path={`${rowPath}.allowance`} problems={form.problems}>
              <CodeSelect
                value={rule.allowance}
                set="allowance"
                vocabulary={vocabulary}
                locked={form.locked}
                onChange={(allowance) => patchAmendment({ rules: replaceAt(amendment.rules, index, { allowance }) })}
              />
            </Field>
          </RowFrame>
        );
      })}
    </DeclarationSectionFrame>
  );
}

// ——一节声明的外框：标题、此刻是留空还是会随发布登记（从草稿算出的事实）、「加一行」与「清空本节」。清空把这一节
// 退回留空——那是回到「未声明」的路，不是把它写成「无」。

function DeclarationSectionFrame({
  title,
  section,
  draft,
  update,
  locked,
  addLabel = '加一行',
  onAdd,
  children,
}: {
  title: string;
  section: DeclarationSection;
  draft: AcceptanceRulePackageDraft;
  update: Update;
  locked: boolean;
  addLabel?: string;
  onAdd?: () => void;
  children: ReactNode;
}) {
  const declared = sectionDeclared(draft, section);
  const clear = () =>
    update((current) => ({ ...current, [section]: emptyAcceptanceRulePackageDraft()[section] }) as AcceptanceRulePackageDraft);
  return (
    <section className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <h3 className={sectionTitle}>{title}</h3>
        <div className="flex items-center gap-2">
          <SectionState declared={declared} />
          {declared ? (
            <Button variant="outline" disabled={locked} onClick={clear}>
              清空本节
            </Button>
          ) : null}
          {onAdd ? (
            <Button variant="outline" disabled={locked} onClick={onAdd}>
              {addLabel}
            </Button>
          ) : null}
        </div>
      </div>
      {children}
    </section>
  );
}

function SectionState({ declared }: { declared: boolean }) {
  return (
    <span className={hint}>{declared ? '本节将随发布登记' : '本节留空，即未声明'}</span>
  );
}

function RowsHeader({ title, locked, onAdd }: { title: string; locked: boolean; onAdd: () => void }) {
  return (
    <div className="flex items-center justify-between mt-1">
      <span className={fieldLabel}>{title}</span>
      <Button variant="outline" disabled={locked} onClick={onAdd}>
        加一行
      </Button>
    </div>
  );
}