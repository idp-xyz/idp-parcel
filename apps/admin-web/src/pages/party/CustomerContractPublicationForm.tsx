import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { accessRefusalTitle, isOperatorRefusal, type OperatorRefusal } from '../../auth/access-refusal';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone } from '../moment';
import { listCommercialPolicies, type CommercialPolicyListResponseBody, listCustomerContracts } from './api';
import { ExistingVersionsNote } from './ExistingVersionsNote';
import { contractKnownVersions } from './known-versions';
import { controlRequirementLabels, labelOf } from './presentation';
import { DeliveryConditionFields } from './DeliveryConditionFields';
import type { NewVersionStart } from './new-version';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { Field, RowFrame, SearchableSelect, fieldLabel, selectClass, useLoaded } from './PublicationFormFields';
import type { DeliveryConditionDraft } from './delivery-condition-section';
import { WallTimeField, customerAccountPickerOptions } from './party-registration-fields';
import { SuggestInput, loadCustomerAccountSuggestions } from './reference-inputs';
import {
  customerContractDeliveryConditionsPath,
  customerContractFieldPaths,
  customerContractLocalProblems,
  emptyBindingDraft,
  emptyCustomerContractDraft,
  payloadOf,
  type BindingMode,
  type ControlBindingDraft,
  type ControlRequirementDraft,
  type CustomerContractDraft,
} from './customer-contract-form';

const loadKnownVersions = () => listCustomerContracts();

/**
 * 客户合同版本的逐字段发布表单（票 admin-write-faces/10；ADR-0101 决定八自裁为逐字段表单 + 约定表可加行）。
 * 五步「表单 → 预览摘要 → 存为待批准 → 批准 → 发布」由 PublicationDraftFlow 走，本组件只摆本册的几格并把
 * 草稿组成载荷（纯函数在 customer-contract-form.ts）。
 *
 * **两层分两节、不合并**（ADR-0115）：0012 的合同正文（接单规则包 + 按费用范围的约定表）与 0007 的合同级
 * 「要不要接受前财务控制」声明是同一份合同的两层话——前者按范围答「用哪份 / 不适用」，后者答整份合同
 * 「要不要」；并成一节会让「不适用」在两个层面上撞成同一个词。第三节是合同层交付条件（0030，票 admin-write-faces/25）：
 * 可缺，一格都不填即这一版没有合同层声明；字段组与产品版本表单共用 DeliveryConditionFields。
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
  /** 基于目录里某一版预填（票 party-commercial-uiux/14）；换一次预填由页面换 key 重建本表单。 */
  start?: NewVersionStart<CustomerContractDraft>;
}

const sectionTitle = 'text-[13px] font-medium text-idpxyz-text';

export function CustomerContractPublicationForm({ onPublished, start }: CustomerContractPublicationFormProps) {
  const [draft, setDraft] = useState<CustomerContractDraft>(start?.draft ?? emptyCustomerContractDraft());
  const rulePackages = useObjectCandidates(loadRulePackages);
  const controlPolicies = useObjectCandidates(loadControlPolicies);

  const patch = (change: Partial<CustomerContractDraft>) => setDraft((current) => ({ ...current, ...change }));
  const patchBinding = (index: number, change: Partial<ControlBindingDraft>) =>
    patch({ bindings: draft.bindings.map((binding, at) => (at === index ? { ...binding, ...change } : binding)) });
  const patchDeliveryConditions = (change: Partial<DeliveryConditionDraft>) =>
    setDraft((current) => ({ ...current, deliveryConditions: { ...current.deliveryConditions, ...change } }));
  // 时刻按操作者的显示时区填、送出 UTC（票 party-commercial-uiux/10），与登记表单同一条换算。
  const timeZone = currentDisplayTimeZone();
  // 这个对象已有哪些版本（票 party-commercial-uiux/15）：读不到就不提示，不报错。
  const knownAnswer = useLoaded(loadKnownVersions);
  const known = knownAnswer?.kind === 'outcome' ? contractKnownVersions(knownAnswer.body) : null;

  return (
    <PublicationDraftFlow
      kind="CUSTOMER_CONTRACT"
      title="发布客户合同版本"
      assemblePayload={() => payloadOf(draft, timeZone)}
      localProblems={customerContractLocalProblems(draft, timeZone)}
      fieldPaths={customerContractFieldPaths(draft)}
      onPublished={onPublished}
      prefilledFrom={start?.from}
      prefilledCaution={start?.caution}
      versionNote={
        <ExistingVersionsNote
          known={known}
          objectId={draft.objectId}
          version={draft.version}
          startsAt={draft.effectiveStartsAt}
          endsAt={draft.effectiveEndsAt}
          timeZone={timeZone}
        />
      }
    >
      {(form) => (
        <div className="flex flex-col gap-5">
          <ShellFields draft={draft} patch={patch} form={form} timeZone={timeZone} />
          <ContractContentFields
            draft={draft}
            patch={patch}
            patchBinding={patchBinding}
            form={form}
            rulePackages={rulePackages}
            controlPolicies={controlPolicies}
          />
          <PreAcceptanceControlFields draft={draft} patch={patch} form={form} />
          <DeliveryConditionFields
            root={customerContractDeliveryConditionsPath}
            layer="contract"
            draft={draft.deliveryConditions}
            form={form}
            onPatch={patchDeliveryConditions}
          />
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
  timeZone,
}: {
  draft: CustomerContractDraft;
  patch: (change: Partial<CustomerContractDraft>) => void;
  form: PublicationFormContext;
  timeZone: string;
}) {
  return (
    <section className="flex flex-col gap-2">
      <h3 className={sectionTitle}>版本信息</h3>
      <div className="grid grid-cols-2 gap-3">
        <Field label="合同编号 *" path="objectId" problems={form.problems}>
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
            placeholder="如 v1；改条款就用下一个版本号"
            onChange={(event) => patch({ version: event.target.value })}
          />
        </Field>
        <Field label="适用范围 *" path="scope" problems={form.problems}>
          <Input
            value={draft.scope}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="这份合同管哪些业务，按公司约定的写法填"
            onChange={(event) => patch({ scope: event.target.value })}
          />
        </Field>
        <div />
        <WallTimeField
          label="开始时间 *"
          path="effectiveStartsAt"
          problems={form.problems}
          value={draft.effectiveStartsAt}
          locked={form.locked}
          timeZone={timeZone}
          onChange={(value) => patch({ effectiveStartsAt: value })}
        />
        <WallTimeField
          label="结束时间"
          path="effectiveEndsAt"
          problems={form.problems}
          value={draft.effectiveEndsAt}
          locked={form.locked}
          timeZone={timeZone}
          note="留空即长期有效。"
          onChange={(value) => patch({ effectiveEndsAt: value })}
        />
      </div>
      {form.versionNote}
      <p className="text-[11px] text-idpxyz-textMuted">
        开始时间晚于现在也可以先保存和批准；到了开始时间再发布即可。
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
      <h3 className={sectionTitle}>合同条款：接单规则与按费用范围的财务控制约定</h3>
      {/* 解析时只有键上那一个客户账户的合同参选。账户册读口分页，所以是能直接输入的格、带第一页候选作提示，不做成
          只列得出第一页的选单（票 party-commercial-uiux/13）。 */}
      <SuggestInput
        label="客户账户 *"
        path="customerContract.contractContent.customerAccount"
        problems={form.problems}
        value={draft.customerAccount}
        locked={form.locked}
        onChange={(customerAccount) => patch({ customerAccount })}
        load={loadCustomerAccountSuggestions}
        optionsOf={customerAccountPickerOptions}
        readFace="货主客户账户册"
        placeholder="这份合同属于哪个客户账户"
      />
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
        <span className={fieldLabel}>按费用范围的约定（每个费用范围指名一份策略，或写明不适用）</span>
        <Button
          variant="outline"
          disabled={form.locked}
          onClick={() => patch({ bindings: [...draft.bindings, emptyBindingDraft()] })}
        >
          加一行
        </Button>
      </div>
      {/* 两格都空、同一范围写两行、策略填了册上没有的标识，都照样送上去由服务端答，表单不代判。 */}
      <p className="text-[11px] text-idpxyz-textMuted">
        每行只填其中一项：指名一份接受前财务控制策略，或写明不适用的依据；同一费用范围只写一行。一行都不加也可以，表示没有对任何费用范围作约定——这时查这些费用范围得到的是「不存在」，不是「不适用」。
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
                    placeholder="为什么这个费用范围不需要接受前财务控制"
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
      <h3 className={sectionTitle}>这份合同接单前要不要做财务控制</h3>
      {/* 「不适用」必带依据：缺依据的不适用与一次默认放行分不开；「要求」不得带依据。两条都由服务端裁。 */}
      <p className="text-[11px] text-idpxyz-textMuted">
        这里回答整份合同接单前要不要做财务控制（预付冻结、信用校验）；上面的约定表按费用范围回答用哪份策略。选「不适用」要写明商业依据。
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
  | OperatorRefusal
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
  if (isOperatorRefusal(answer)) return { kind: answer.kind };
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
    return (
      <>
        {/* 手填过、册上又没有的标识由选单保留为一项，免得它被静默吞掉——它对不对由服务端答。 */}
        <SearchableSelect
          options={candidates.objectIds.map((objectId) => ({ value: objectId, label: objectId }))}
          value={value}
          disabled={locked}
          onChange={onChange}
        />
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
            ? `暂时读不到${registerName}（接入渠道未配置），先手填标识；标识是否在册由服务端在发布时核对。`
            : candidates.kind === 'unavailable'
              ? `暂时读不到${registerName}，先手填标识；标识是否在册由服务端在发布时核对。`
              : `暂时读不到${registerName}（${accessRefusalTitle(candidates.kind)}），先手填标识；标识是否在册由服务端在发布时核对。`}
      </span>
    </>
  );
}