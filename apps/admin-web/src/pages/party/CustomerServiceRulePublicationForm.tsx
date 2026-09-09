import { useEffect, useState, type ReactNode } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import { claimDeadlineKindLabels } from './presentation';
import {
  fetchPublicationVocabulary,
  vocabularyOptions,
  type PublicationVocabularyResponseBody,
} from './publication-draft-api';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import {
  emptyServiceRuleDraft,
  serviceRuleCodesOf,
  serviceRuleFieldPaths,
  serviceRuleLocalProblems,
  serviceRulePayloadOf,
  withClaimDeadlineRowAdded,
  withClaimDeadlineRowPatched,
  withClaimDeadlineRowRemoved,
  withMinimumMaterialsRowAdded,
  withMinimumMaterialsRowPatched,
  withMinimumMaterialsRowRemoved,
  type ClaimDeadlineDraft,
  type MinimumMaterialsDraft,
  type ServiceRuleAppliesTo,
  type ServiceRuleDraft,
} from './customer-service-rule-form';

/**
 * 客户服务规则版本的逐字段表单（票 admin-write-faces/18；ADR-0101 决定八逐册裁形）。
 *
 * **为什么是逐字段表单、两张子表可加行**：低频、运营配置员操作、正文是两个引用加两张几行的表，不是矩阵。五步
 * （预览摘要 → 存为待批准 → 批准 → 发布）由公共半边 PublicationDraftFlow 走，本组件只摆版本壳五格与规则正文，
 * 草稿 → 载荷在 customer-service-rule-form.ts。
 *
 * **适用对象是二选一控件，恰一由服务端裁**：控件让操作者一次只能说「随某个服务产品」或「随某个客户合同」，那是
 * 呈现不是裁门——两键都填在这个控件上表达不出来，服务端对「两键恰一」那一格（customerServiceRule.applicability）
 * 的话照样挂在这里。同一个标识同时进壳上的指名引用（references.SERVICE_PRODUCT / CUSTOMER_CONTRACT），壳与正文
 * 的适用一致由服务端在发布写入前核，表单不替操作者抄第二遍。起始未选，不预选任何一种。
 *
 * **期限种类只从词表选，其余手填**：期限种类的下拉只吃服务端词表读口（票 20，`kind=CUSTOMER_SERVICE_RULE` 的
 * `kind` 一集）答的码 × 本页中文词表，表单不内置任何一格、不预选。起算事件、日历、索赔类型与材料都是开放引用串
 * 手填（解释权在可见性与异常那侧，本台不替它们造词）；时长是整数格。词表在未配置那堵墙前（403）或读不到时下拉
 * 显占位、不自造码，送预览会由服务端点名那一格。
 *
 * **表单不算摘要、不收也不送批准人、不裁任何门**（伞票 07 硬句）：空字段、集外的码、引用在不在册一律送上去，
 * 预览口逐格 problems 回来挂到对应格旁；两项合起来至少一行、每一种期限与每一种索赔类型至多一行是跨行的门，由
 * 构造门在预览上答成成因。本地只拦编不进 JSON 整数的时长文本（localProblems）。两张表起始零行：「这一版对期限
 * 无客户差异」是正文说得出的真话，预开一行会替操作者预设「这张表有内容」。
 */
export interface CustomerServiceRulePublicationFormProps {
  /** 载体到达「发布」那一步时回调，政策页借它切到客户服务规则册并重读。 */
  onPublished?: () => void;
}

const fieldLabel = 'block text-[12px] text-idpxyz-textMuted mb-1';
const selectClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 text-[13px] ' +
  'text-idpxyz-text focus:outline-none focus:border-idpxyz-accent disabled:opacity-60';
const textareaClass = `${selectClass} font-mono min-h-[72px]`;
const momentPlaceholder = 'RFC 3339 或 YYYY-MM-DD（只到天补成当天零点 UTC）';
const bodyPath = 'customerServiceRule';

// 二选一控件的两格：取值就是载荷键名原词，与 customer-service-rule-form 的 ServiceRuleAppliesTo 同一套词。这是表单
// 自己的形状（正文两键之一），不是领域封闭集，所以不经词表。
const appliesToChoices: { value: Exclude<ServiceRuleAppliesTo, ''>; label: string; noun: string; shellKey: string }[] = [
  { value: 'serviceProduct', label: '随某个服务产品适用', noun: '服务产品', shellKey: 'SERVICE_PRODUCT' },
  { value: 'customerContract', label: '随某个客户合同适用', noun: '客户合同', shellKey: 'CUSTOMER_CONTRACT' },
];

export function CustomerServiceRulePublicationForm({ onPublished }: CustomerServiceRulePublicationFormProps) {
  const [draft, setDraft] = useState<ServiceRuleDraft>(emptyServiceRuleDraft());
  const patch = (change: Partial<ServiceRuleDraft>) => setDraft((current) => ({ ...current, ...change }));
  const patchDeadline = (index: number, change: Partial<ClaimDeadlineDraft>) =>
    setDraft((current) => withClaimDeadlineRowPatched(current, index, change));
  const patchMaterials = (index: number, change: Partial<MinimumMaterialsDraft>) =>
    setDraft((current) => withMinimumMaterialsRowPatched(current, index, change));
  // 词表读一次、只读一次：表单打开时取一份，行再多也不重取。
  const vocabulary = useLoaded(loadServiceRuleVocabulary);
  const sets = vocabulary?.kind === 'outcome' ? vocabulary.body.sets : null;
  const kindCodes = sets === null ? null : serviceRuleCodesOf(sets, 'kind');
  const chosen = appliesToChoices.find((choice) => choice.value === draft.appliesTo) ?? null;

  return (
    <PublicationDraftFlow
      kind="CUSTOMER_SERVICE_RULE"
      title="发布客户服务规则版本"
      assemblePayload={() => serviceRulePayloadOf(draft)}
      localProblems={serviceRuleLocalProblems(draft)}
      fieldPaths={serviceRuleFieldPaths(draft)}
      onPublished={onPublished}
    >
      {({ problems, locked }: PublicationFormContext) => (
        <div className="flex flex-col gap-5">
          <section className="flex flex-col gap-3">
            <h3 className="text-[13px] font-medium text-idpxyz-text">版本壳</h3>
            <p className="text-[11px] text-idpxyz-textMuted">
              壳上的适用范围与区间是<strong>版本</strong>的（登记册逐列比对的项）；区间上界留空即持续有效。壳上的指名引用
              不在这里另填：下方适用对象选定的那个服务产品或客户合同就是壳上指名的对象，表单只送一份，服务端核两处一致。
            </p>
            <div className="grid grid-cols-2 gap-3">
              <Field label="规则对象标识 *" path="objectId" problems={problems}>
                <Input
                  value={draft.objectId}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  onChange={(event) => patch({ objectId: event.target.value })}
                />
              </Field>
              <Field label="版本号 *" path="version" problems={problems}>
                <Input
                  value={draft.version}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  onChange={(event) => patch({ version: event.target.value })}
                />
              </Field>
              <Field label="版本适用范围 *" path="scope" problems={problems}>
                <Input
                  value={draft.scope}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  onChange={(event) => patch({ scope: event.target.value })}
                />
              </Field>
              <div />
              <Field label="版本生效起点 *" path="effectiveStartsAt" problems={problems}>
                <Input
                  value={draft.effectiveStartsAt}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder={momentPlaceholder}
                  onChange={(event) => patch({ effectiveStartsAt: event.target.value })}
                />
              </Field>
              <Field label="版本生效止点（可空）" path="effectiveEndsAt" problems={problems}>
                <Input
                  value={draft.effectiveEndsAt}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder="留空即持续有效"
                  onChange={(event) => patch({ effectiveEndsAt: event.target.value })}
                />
              </Field>
            </div>
          </section>

          <section className="flex flex-col gap-3">
            <h3 className="text-[13px] font-medium text-idpxyz-text">规则正文（0023）</h3>
            <p className="text-[11px] text-idpxyz-textMuted">
              正文是「挂在哪个对象上」「责任方」「适用范围」三格加两张表：索赔期限按种类成行，最低材料按索赔类型成行。
              适用对象恰一、两张表合起来至少一行、每一种期限与每一种索赔类型至多一行——都由服务端答，这里不拦。正文只
              发布本册这一侧；索赔类型与材料目录、起算事件的解释归可见性与异常那侧，这里按引用原词填。
            </p>
            <div className="grid grid-cols-2 gap-3">
              <Field label="适用对象 *" path={`${bodyPath}.applicability`} problems={problems}>
                <select
                  className={selectClass}
                  value={draft.appliesTo}
                  disabled={locked}
                  onChange={(event) => patch({ appliesTo: event.target.value as ServiceRuleAppliesTo })}
                >
                  <option value="">未选</option>
                  {appliesToChoices.map((choice) => (
                    <option key={choice.value} value={choice.value}>
                      {choice.label} · {choice.value}
                    </option>
                  ))}
                </select>
              </Field>
              {chosen === null ? (
                <Field label="适用对象标识 *" path={`${bodyPath}.<serviceProduct | customerContract>`} problems={problems}>
                  <Input value="" disabled className="font-mono text-[13px]" placeholder="先在左侧选定适用对象是产品还是合同" />
                </Field>
              ) : (
                <Field label="适用对象标识 *" path={`${bodyPath}.${chosen.value}`} problems={problems}>
                  <Input
                    value={draft.appliesToId}
                    disabled={locked}
                    className="font-mono text-[13px]"
                    placeholder={`${chosen.noun}的对象标识（同一个值进壳上 references.${chosen.shellKey}）`}
                    onChange={(event) => patch({ appliesToId: event.target.value })}
                  />
                  {(problems[`references.${chosen.shellKey}`] ?? []).map((line) => (
                    <span key={line} className="block text-[11px] text-idpxyz-danger mt-1">
                      壳上引用 references.{chosen.shellKey}：{line}
                    </span>
                  ))}
                </Field>
              )}
              <Field label="责任方 *" path={`${bodyPath}.responsible`} problems={problems}>
                <Input
                  value={draft.responsible}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder="参与方标识（开放引用，不校验存在性）"
                  onChange={(event) => patch({ responsible: event.target.value })}
                />
              </Field>
              <Field label="规则适用范围 *" path={`${bodyPath}.scope`} problems={problems}>
                <Input
                  value={draft.ruleScope}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder="正文自己的适用范围引用，与壳上的版本范围各是各的格"
                  onChange={(event) => patch({ ruleScope: event.target.value })}
                />
              </Field>
            </div>

            <div className="flex items-center justify-between">
              <h4 className="text-[12px] font-medium text-idpxyz-text">索赔期限（{draft.claimDeadlines.length} 行）</h4>
              <Button variant="outline" disabled={locked} onClick={() => setDraft(withClaimDeadlineRowAdded)}>
                加一行
              </Button>
            </div>
            {draft.claimDeadlines.length === 0 ? (
              <p className="text-[11px] text-idpxyz-textMuted">
                零行。这张表可以是空的——只要下方最低材料表不也是空的；两张都空由服务端在预览上答成因。每一种期限至多一行。
              </p>
            ) : null}
            {draft.claimDeadlines.map((row, index) => (
              <ClaimDeadlineRow
                key={index}
                index={index}
                row={row}
                problems={problems}
                locked={locked}
                kindCodes={kindCodes}
                vocabulary={vocabulary}
                onChange={(change) => patchDeadline(index, change)}
                onRemove={() => setDraft((current) => withClaimDeadlineRowRemoved(current, index))}
              />
            ))}

            <div className="flex items-center justify-between">
              <h4 className="text-[12px] font-medium text-idpxyz-text">最低材料（{draft.minimumMaterials.length} 行）</h4>
              <Button variant="outline" disabled={locked} onClick={() => setDraft(withMinimumMaterialsRowAdded)}>
                加一行
              </Button>
            </div>
            {draft.minimumMaterials.length === 0 ? (
              <p className="text-[11px] text-idpxyz-textMuted">
                零行。这张表可以是空的——只要上方索赔期限表不也是空的。每一种索赔类型至多一行，每行至少一项材料且不重复，都由服务端答。
              </p>
            ) : null}
            {draft.minimumMaterials.map((row, index) => (
              <MinimumMaterialsRow
                key={index}
                index={index}
                row={row}
                problems={problems}
                locked={locked}
                onChange={(change) => patchMaterials(index, change)}
                onRemove={() => setDraft((current) => withMinimumMaterialsRowRemoved(current, index))}
              />
            ))}
          </section>
        </div>
      )}
    </PublicationDraftFlow>
  );
}

// 一行索赔期限：一个下拉 + 两个引用串 + 一个整数格，外加删行；行本身（claimDeadlines[i]）被点名时显在行下。
function ClaimDeadlineRow({
  index,
  row,
  problems,
  locked,
  kindCodes,
  vocabulary,
  onChange,
  onRemove,
}: {
  index: number;
  row: ClaimDeadlineDraft;
  problems: Record<string, string[]>;
  locked: boolean;
  kindCodes: string[] | null;
  vocabulary: ApiResult<PublicationVocabularyResponseBody> | null;
  onChange: (change: Partial<ClaimDeadlineDraft>) => void;
  onRemove: () => void;
}) {
  const rowPath = `${bodyPath}.claimDeadlines[${index}]`;
  const rowLines = problems[rowPath] ?? [];
  return (
    <div className="rounded border border-idpxyz-border p-3 flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <span className="text-[12px] text-idpxyz-textMuted">
          期限第 {index + 1} 行 <span className="font-mono text-[10px]">{rowPath}</span>
        </span>
        <Button variant="outline" disabled={locked} onClick={onRemove}>
          删这一行
        </Button>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <VocabularySelect
          label="期限种类 *"
          path={`${rowPath}.kind`}
          setName="kind"
          codes={kindCodes}
          vocabulary={vocabulary}
          labels={claimDeadlineKindLabels}
          problems={problems}
          value={row.kind}
          locked={locked}
          onChange={(kind) => onChange({ kind })}
        />
        <Field label="起算事件 *" path={`${rowPath}.startEvent`} problems={problems}>
          <Input
            value={row.startEvent}
            disabled={locked}
            className="font-mono text-[13px]"
            placeholder="起算事件引用串（开放引用，解释归可见性与异常）"
            onChange={(event) => onChange({ startEvent: event.target.value })}
          />
        </Field>
        <Field label="时长（天）*" path={`${rowPath}.days`} problems={problems}>
          <Input
            value={row.days}
            disabled={locked}
            className="font-mono text-[13px]"
            placeholder="正整数天；按哪份日历数由右侧日历引用说"
            onChange={(event) => onChange({ days: event.target.value })}
          />
        </Field>
        <Field label="业务日历 *" path={`${rowPath}.calendar`} problems={problems}>
          <Input
            value={row.calendar}
            disabled={locked}
            className="font-mono text-[13px]"
            placeholder="业务日历或时区引用串（日历内容属实例半边）"
            onChange={(event) => onChange({ calendar: event.target.value })}
          />
        </Field>
      </div>
      {rowLines.map((line) => (
        <span key={line} className="block text-[11px] text-idpxyz-danger">
          {line}
        </span>
      ))}
    </div>
  );
}

// 一行最低材料：一个索赔类型引用串 + 多行文本的材料清单（一行一项），外加删行；行本身与清单某一项被点名时各显各处。
function MinimumMaterialsRow({
  index,
  row,
  problems,
  locked,
  onChange,
  onRemove,
}: {
  index: number;
  row: MinimumMaterialsDraft;
  problems: Record<string, string[]>;
  locked: boolean;
  onChange: (change: Partial<MinimumMaterialsDraft>) => void;
  onRemove: () => void;
}) {
  const rowPath = `${bodyPath}.minimumMaterials[${index}]`;
  const rowLines = problems[rowPath] ?? [];
  // 服务端对清单某一项的话落在 materials[j]；清单在这里是一块多行文本，逐项的话汇到文本框下、带上项号。
  const itemLines = Object.entries(problems)
    .filter(([path]) => path.startsWith(`${rowPath}.materials[`))
    .flatMap(([path, lines]) => lines.map((line) => `${path.slice(rowPath.length + 1)}：${line}`));
  return (
    <div className="rounded border border-idpxyz-border p-3 flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <span className="text-[12px] text-idpxyz-textMuted">
          材料第 {index + 1} 行 <span className="font-mono text-[10px]">{rowPath}</span>
        </span>
        <Button variant="outline" disabled={locked} onClick={onRemove}>
          删这一行
        </Button>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="索赔类型 *" path={`${rowPath}.claimKind`} problems={problems}>
          <Input
            value={row.claimKind}
            disabled={locked}
            className="font-mono text-[13px]"
            placeholder="索赔类型引用串（类型目录归可见性与异常）"
            onChange={(event) => onChange({ claimKind: event.target.value })}
          />
        </Field>
        <Field label="材料清单 *（一行一项）" path={`${rowPath}.materials[j]`} problems={problems}>
          <textarea
            className={textareaClass}
            value={row.materials}
            disabled={locked}
            placeholder={'材料条目引用串（材料目录归可见性与异常），一行一项；空行不算项'}
            onChange={(event) => onChange({ materials: event.target.value })}
          />
          {itemLines.map((line) => (
            <span key={line} className="block text-[11px] text-idpxyz-danger mt-1">
              {line}
            </span>
          ))}
        </Field>
      </div>
      {rowLines.map((line) => (
        <span key={line} className="block text-[11px] text-idpxyz-danger">
          {line}
        </span>
      ))}
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
  const lines = problems[path] ?? [];
  return (
    <label className="block">
      <span className={fieldLabel}>
        {label} <span className="font-mono text-[10px]">{path}</span>
      </span>
      {children}
      {lines.map((line) => (
        <span key={line} className="block text-[11px] text-idpxyz-danger mt-1">
          {line}
        </span>
      ))}
    </label>
  );
}

// 读一次、只读一次：词表是目录，表单打开时取一份，不随每次击键重取。
function useLoaded<Body>(load: () => Promise<ApiResult<Body>>): ApiResult<Body> | null {
  const [answer, setAnswer] = useState<ApiResult<Body> | null>(null);
  useEffect(() => {
    let cancelled = false;
    void load().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [load]);
  return answer;
}

/**
 * 封闭集下拉：选项 = 服务端词表（票 20）那一集的码 × 本页中文词表；词表没收录的码原样示出。词表在未配置那堵墙前
 * （403）、调用方问题、未形成答案、没到达、或答复里没有这一集时都**显占位不显码**——表单不内置任何一格，内置一份
 * 就是同一封闭集的第二份写法。不预选：「未选」是一个空值状态，不是默认值。
 */
function VocabularySelect({
  label,
  path,
  setName,
  codes,
  vocabulary,
  labels,
  problems,
  value,
  locked,
  onChange,
}: {
  label: string;
  path: string;
  setName: string;
  codes: string[] | null;
  vocabulary: ApiResult<PublicationVocabularyResponseBody> | null;
  labels: Record<string, string>;
  problems: Record<string, string[]>;
  value: string;
  locked: boolean;
  onChange: (value: string) => void;
}) {
  if (codes !== null) {
    const options = vocabularyOptions(codes, labels);
    const known = options.some((option) => option.value === value);
    return (
      <Field label={label} path={path} problems={problems}>
        <select className={selectClass} value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
          <option value="">未选</option>
          {!known && value !== '' ? <option value={value}>{value}（不在词表上）</option> : null}
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label} · {option.value}
            </option>
          ))}
        </select>
        {options.length === 0 ? (
          <span className="block text-[11px] text-idpxyz-textMuted mt-1">服务端词表 {setName} 一集今天没有码；表单不自造。</span>
        ) : null}
      </Field>
    );
  }

  return (
    <Field label={label} path={path} problems={problems}>
      <select className={selectClass} value="" disabled>
        <option value="">{vocabularyPlaceholder(vocabulary, setName)}</option>
      </select>
      <span className="block text-[11px] text-idpxyz-textMuted mt-1">
        码只从服务端词表读口取（GET /commercial-publication-vocabularies?kind=CUSTOMER_SERVICE_RULE），
        表单不内置枚举、不自造码；词表就绪前这一格选不了，送预览会由服务端点名。
      </span>
    </Field>
  );
}

// 稳定的函数引用：useLoaded 以它为依赖，写成内联箭头会每次渲染重取。
function loadServiceRuleVocabulary(): Promise<ApiResult<PublicationVocabularyResponseBody>> {
  return fetchPublicationVocabulary('CUSTOMER_SERVICE_RULE');
}

function vocabularyPlaceholder(answer: ApiResult<PublicationVocabularyResponseBody> | null, setName: string): string {
  if (answer === null) return '正在读词表…';
  switch (answer.kind) {
    case 'outcome':
      return `词表未就绪（服务端没有 ${setName} 一集）`;
    case 'unconfigured':
      return '词表未就绪（接入渠道未配置，403）';
    case 'callerProblem':
      return `词表未就绪（调用方式问题，HTTP ${answer.status}）`;
    case 'noAnswer':
      return `词表未就绪（服务端未形成答案，HTTP ${answer.status}）`;
    case 'transport':
      return '词表未就绪（请求未到达 parcel-api）';
  }
}
