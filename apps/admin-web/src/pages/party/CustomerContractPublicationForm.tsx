import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import { listCommercialPolicies, type CommercialPolicyListResponseBody } from './api';
import { controlRequirementLabels, labelOf } from './presentation';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { Field, RowFrame, fieldLabel, selectClass, useLoaded } from './PublicationFormFields';
import {
  customerContractFieldPaths,
  emptyBindingDraft,
  emptyCustomerContractDraft,
  payloadOf,
  type BindingMode,
  type ControlBindingDraft,
  type ControlRequirementDraft,
  type CustomerContractDraft,
} from './customer-contract-form';

/**
 * 客户合同版本的逐字段发布表单（票 admin-write-faces/10；ADR-0101 决定八自裁为逐字段表单 + 约定表可加行）。
 * 五步「表单 → 预览摘要 → 存为待批准 → 批准 → 发布」由 PublicationDraftFlow 走，本组件只摆本册的几格并把
 * 草稿组成载荷（纯函数在 customer-contract-form.ts）。
 *
 * **两层分两节、不合并**（ADR-0115）：0012 的合同正文（接单规则包 + 按费用范围的约定表）与 0007 的合同级
 * 「要不要接受前财务控制」声明是同一份合同的两层话——前者按范围答「用哪份 / 不适用」，后者答整份合同
 * 「要不要」；并成一节会让「不适用」在两个层面上撞成同一个词。
 *
 * **表单不算摘要、不裁任何门、不代判**（伞票 07 硬句；票 10 硬句）：约定行的「指名策略 / 显式不适用」与合同级
 * 的「要求 / 不适用」都是二选一控件，没选、选了没填照样送上去，答回来的是构造门对那一行 / 那一节的拒绝；
 * 同一范围约定两次、`不适用`缺依据由领域在预览上答成`未受理`带成因。**策略侧没有「无控制」选项**
 * （ADR-0115 Decision 一）：「明确无控制」只能经这两层声明表达。
 *
 * 规则包从接单规则包册选、策略从接受前财务控制策略册选，都走既有的 listCommercialPolicies 读面（票 06 落的
 * 第九册），不新开口；读面在未配置那堵墙前时退回手填（判据同 pricing 的 QuoteBasisField），表单不因此变死。
 */
export interface CustomerContractPublicationFormProps {
  /** 载体到达「发布」那一步时回调，页面借它刷同页目录读面（含绑定列）。 */
  onPublished?: () => void;
}

const sectionTitle = 'text-[13px] font-medium text-idpxyz-text';

export function CustomerContractPublicationForm({ onPublished }: CustomerContractPublicationFormProps) {
  const [draft, setDraft] = useState<CustomerContractDraft>(emptyCustomerContractDraft());
  const rulePackages = useObjectCandidates(loadRulePackages);
  const controlPolicies = useObjectCandidates(loadControlPolicies);

  const patch = (change: Partial<CustomerContractDraft>) => setDraft((current) => ({ ...current, ...change }));
  const patchBinding = (index: number, change: Partial<ControlBindingDraft>) =>
    patch({ bindings: draft.bindings.map((binding, at) => (at === index ? { ...binding, ...change } : binding)) });

  return (
    <PublicationDraftFlow
      kind="CUSTOMER_CONTRACT"
      title="发布客户合同版本"
      assemblePayload={() => payloadOf(draft)}
      fieldPaths={customerContractFieldPaths(draft)}
      onPublished={onPublished}
    >
      {(form) => (
        <div className="flex flex-col gap-5">
          <ShellFields draft={draft} patch={patch} form={form} />
          <ContractContentFields
            draft={draft}
            patch={patch}
            patchBinding={patchBinding}
            form={form}
            rulePackages={rulePackages}
            controlPolicies={controlPolicies}
          />
          <PreAcceptanceControlFields draft={draft} patch={patch} form={form} />
        </div>
      )}
    </PublicationDraftFlow>
  );
}

// ——版本壳：身份三元（对象标识 / 版本号 / 适用范围）与有效区间。租户不在这里：它从操作者信封来。

function ShellFields({
  draft,
  patch,
  form,
}: {
  draft: CustomerContractDraft;
  patch: (change: Partial<CustomerContractDraft>) => void;
  form: PublicationFormContext;
}) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className={sectionTitle}>版本壳</h3>
      <div className="grid grid-cols-2 gap-3">
        <Field label="合同对象标识 *" path="objectId" problems={form.problems}>
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
            placeholder="RFC 3339，如 2026-10-01T00:00:00Z"
            onChange={(event) => patch({ effectiveStartsAt: event.target.value })}
          />
        </Field>
        <Field label="生效止点（留空即无上界）" path="effectiveEndsAt" problems={form.problems}>
          <Input
            value={draft.effectiveEndsAt}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="RFC 3339；留空即无上界"
            onChange={(event) => patch({ effectiveEndsAt: event.target.value })}
          />
        </Field>
      </div>
      <p className="text-[11px] text-idpxyz-textMuted">
        起点晚于此刻时载体可存、可批，发布会答「等待生效边界」——到界再来发布，不是失败。
      </p>
    </section>
  );
}

// ——合同正文（0012）：接单规则包 + 按费用范围的约定表。规则包同时进壳上的指名引用与正文，两处必须相等由服务端核。

function ContractContentFields({
  draft,
  patch,
  patchBinding,
  form,
  rulePackages,
  controlPolicies,
}: {
  draft: CustomerContractDraft;
  patch: (change: Partial<CustomerContractDraft>) => void;
  patchBinding: (index: number, change: Partial<ControlBindingDraft>) => void;
  form: PublicationFormContext;
  rulePackages: ObjectCandidates;
  controlPolicies: ObjectCandidates;
}) {
  const bindingsBase = 'customerContract.contractContent.bindings';
  return (
    <section className="flex flex-col gap-2">
      <h3 className={sectionTitle}>合同正文：接单规则包与按费用范围的接受前财务控制约定</h3>
      <Field
        label="接单规则包 *"
        path="customerContract.contractContent.rulePackage"
        alsoPaths={['references.ACCEPTANCE_RULE_PACKAGE', 'customerContract.contractContent']}
        problems={form.problems}
      >
        <ObjectPicker
          value={draft.rulePackage}
          candidates={rulePackages}
          locked={form.locked}
          registerName="接单规则包册"
          onChange={(rulePackage) => patch({ rulePackage })}
        />
      </Field>

      <div className="flex items-center justify-between mt-1">
        <span className={fieldLabel}>按费用范围的约定（费用范围 × 指名策略 / 显式不适用）</span>
        <Button
          variant="outline"
          disabled={form.locked}
          onClick={() => patch({ bindings: [...draft.bindings, emptyBindingDraft()] })}
        >
          加一行
        </Button>
      </div>
      <p className="text-[11px] text-idpxyz-textMuted">
        每行恰一格：指名一份接受前财务控制策略，或显式写下不适用依据。两格都空、同一范围写两行、策略侧填了
        册上没有的标识——都照样送上去由服务端答，表单不代判。零行也是合法正文（「已登记，未对任何费用范围作约定」），
        但届时该范围答「不存在」而不是「不适用」。
      </p>
      <div className="flex flex-col gap-2">
        {draft.bindings.map((binding, index) => {
          const row = `${bindingsBase}[${index}]`;
          return (
            <RowFrame
              key={index}
              path={row}
              problems={form.problems}
              locked={form.locked}
              columns="grid-cols-[1fr_auto_1fr_auto]"
              onRemove={() => patch({ bindings: draft.bindings.filter((_, at) => at !== index) })}
            >
              <Field label="费用范围 *" path={`${row}.chargeScope`} problems={form.problems}>
                <Input
                  value={binding.chargeScope}
                  readOnly={form.locked}
                  className="font-mono text-[12px]"
                  placeholder="费用范围引用"
                  onChange={(event) => patchBinding(index, { chargeScope: event.target.value })}
                />
              </Field>
              <div>
                <span className={fieldLabel}>约定 *</span>
                <BindingModePicker
                  value={binding.mode}
                  locked={form.locked}
                  onChange={(mode) => patchBinding(index, { mode })}
                />
              </div>
              {/* 两格只显一格，隐着的那格认领了就要有处显（票 22 判据 3）：它的问题由显着的那格代显，服务端点到哪一格都不被吞。 */}
              {binding.mode === 'inapplicable' ? (
                <Field
                  label="不适用依据 *"
                  path={`${row}.inapplicabilityBasis`}
                  alsoPaths={[`${row}.policy`]}
                  problems={form.problems}
                >
                  <Input
                    value={binding.inapplicabilityBasis}
                    readOnly={form.locked}
                    className="font-mono text-[12px]"
                    placeholder="凭什么该范围不带接受前财务控制"
                    onChange={(event) => patchBinding(index, { inapplicabilityBasis: event.target.value })}
                  />
                </Field>
              ) : (
                <Field
                  label="接受前财务控制策略 *"
                  path={`${row}.policy`}
                  alsoPaths={[`${row}.inapplicabilityBasis`]}
                  problems={form.problems}
                >
                  <ObjectPicker
                    value={binding.policy}
                    candidates={controlPolicies}
                    locked={form.locked || binding.mode !== 'policy'}
                    registerName="接受前财务控制策略册"
                    onChange={(policy) => patchBinding(index, { policy })}
                  />
                </Field>
              )}
            </RowFrame>
          );
        })}
      </div>
    </section>
  );
}

// 行的二选一。第三个选项不存在——策略侧不得出现「无控制」（ADR-0115 Decision 一），不适用是另一格而不是一种策略。
function BindingModePicker({
  value,
  locked,
  onChange,
}: {
  value: BindingMode;
  locked: boolean;
  onChange: (mode: BindingMode) => void;
}) {
  const modes: { mode: Exclude<BindingMode, ''>; label: string }[] = [
    { mode: 'policy', label: '指名策略' },
    { mode: 'inapplicable', label: '显式不适用' },
  ];
  return (
    <div className="flex items-center gap-1">
      {modes.map(({ mode, label }) => (
        <Button
          key={mode}
          variant={value === mode ? 'default' : 'outline'}
          disabled={locked}
          onClick={() => onChange(mode)}
        >
          {label}
        </Button>
      ))}
    </div>
  );
}

// ——合同级声明（0007）：整份合同「要不要接受前财务控制」，依据只在`不适用`时在场。

function PreAcceptanceControlFields({
  draft,
  patch,
  form,
}: {
  draft: CustomerContractDraft;
  patch: (change: Partial<CustomerContractDraft>) => void;
  form: PublicationFormContext;
}) {
  const requirements: Exclude<ControlRequirementDraft, ''>[] = ['REQUIRED', 'NOT_APPLICABLE'];
  const basisPath = 'customerContract.preAcceptanceControl.notApplicableBasis';
  const basisShown = draft.controlRequirement === 'NOT_APPLICABLE';
  return (
    <section className="flex flex-col gap-2">
      <h3 className={sectionTitle}>合同级声明：这份合同要不要接受前财务控制</h3>
      <p className="text-[11px] text-idpxyz-textMuted">
        与上面的约定表是两层：这里答整份合同「要不要」，约定表按范围答「用哪份 / 不适用」。`不适用`必带商业依据
        ——缺依据的不适用与一次默认放行分不开；`要求`不得带依据。两条都由服务端裁。
      </p>
      <div className="grid grid-cols-[auto_1fr] gap-3 items-start">
        {/* 依据格只在「不适用」时显；隐着时它认领的路径由「要求」这格代显（票 22 判据 3），显着时各显各的、不重复。 */}
        <Field
          label="要求 *"
          path="customerContract.preAcceptanceControl.requirement"
          alsoPaths={basisShown ? [] : [basisPath]}
          problems={form.problems}
          as="div"
        >
          <div className="flex items-center gap-1">
            {requirements.map((requirement) => (
              <Button
                key={requirement}
                variant={draft.controlRequirement === requirement ? 'default' : 'outline'}
                disabled={form.locked}
                onClick={() => patch({ controlRequirement: requirement })}
              >
                {labelOf(controlRequirementLabels, requirement)}
              </Button>
            ))}
          </div>
        </Field>
        {basisShown ? (
          <Field label="不适用依据 *" path={basisPath} problems={form.problems}>
            <Input
              value={draft.controlNotApplicableBasis}
              readOnly={form.locked}
              className="font-mono text-[13px]"
              placeholder="合同条款或商业依据，如 CONTRACT-CLAUSE/NO-PRE-ACCEPTANCE-CONTROL"
              onChange={(event) => patch({ controlNotApplicableBasis: event.target.value })}
            />
          </Field>
        ) : null}
      </div>
    </section>
  );
}

// ——候选：两本册都走 listCommercialPolicies。选单按对象标识去重——约定指名的是策略**对象**，不是某一版
// （FinancialControlBinding.policy 是 CommercialObjectID），哪一版适用由解析按时点定。

type ObjectCandidates =
  | { kind: 'loading' }
  | { kind: 'listed'; objectIds: string[] }
  | { kind: 'unconfigured' }
  | { kind: 'unavailable' };

// 稳定的函数引用：useLoaded 以它为依赖，写成内联箭头会每次渲染重取。两册各一个，不由 kind 现拼。
function loadRulePackages(): Promise<ApiResult<CommercialPolicyListResponseBody>> {
  return listCommercialPolicies('ACCEPTANCE_RULE_PACKAGE');
}

function loadControlPolicies(): Promise<ApiResult<CommercialPolicyListResponseBody>> {
  return listCommercialPolicies('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY');
}

function useObjectCandidates(load: () => Promise<ApiResult<CommercialPolicyListResponseBody>>): ObjectCandidates {
  const answer = useLoaded(load);
  if (answer === null) return { kind: 'loading' };
  if (answer.kind === 'unconfigured') return { kind: 'unconfigured' };
  if (answer.kind !== 'outcome') return { kind: 'unavailable' };
  // 本表单只读这两册，两册的行都以 objectId 为对象标识；别的册按 kind 判别子分出去（它们的行形状各异）。
  const policies: { objectId: string }[] =
    answer.body.kind === 'ACCEPTANCE_RULE_PACKAGE' || answer.body.kind === 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY'
      ? answer.body.policies
      : [];
  return { kind: 'listed', objectIds: Array.from(new Set(policies.map((policy) => policy.objectId))) };
}

/** 从册选；册读不到时退回手填，并说清为什么（判据同 pricing 的 QuoteBasisField）。 */
function ObjectPicker({
  value,
  candidates,
  locked,
  registerName,
  onChange,
}: {
  value: string;
  candidates: ObjectCandidates;
  locked: boolean;
  registerName: string;
  onChange: (objectId: string) => void;
}) {
  if (candidates.kind === 'listed') {
    // 手填过、册上又没有的标识保留为一项，免得选单把它静默吞掉——它对不对由服务端答。
    const options = value !== '' && !candidates.objectIds.includes(value) ? [value, ...candidates.objectIds] : candidates.objectIds;
    return (
      <>
        <select className={selectClass} value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
          <option value="">未选</option>
          {options.map((objectId) => (
            <option key={objectId} value={objectId}>
              {objectId}
            </option>
          ))}
        </select>
        {candidates.objectIds.length === 0 ? (
          <span className="block text-[11px] text-idpxyz-textMuted mt-1">
            当前租户的{registerName}为空；先发布那一册的版本，再回来指名。
          </span>
        ) : null}
      </>
    );
  }
  return (
    <>
      <Input
        value={value}
        readOnly={locked}
        className="font-mono text-[13px]"
        placeholder="对象标识"
        onChange={(event) => onChange(event.target.value)}
      />
      <span className="block text-[11px] text-idpxyz-textMuted mt-1">
        {candidates.kind === 'loading'
          ? `正在读${registerName}…`
          : candidates.kind === 'unconfigured'
            ? `${registerName}读口在接入渠道未配置那堵墙前（403），先手填；标识是否在册由服务端发布时判。`
            : `${registerName}读不到，先手填；标识是否在册由服务端发布时判。`}
      </span>
    </>
  );
}