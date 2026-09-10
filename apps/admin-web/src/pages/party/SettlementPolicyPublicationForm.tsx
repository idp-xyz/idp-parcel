import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import {
  listBusinessParties,
  listCustomerContracts,
  listGroupLegalEntities,
  type BusinessPartyListResponseBody,
  type GroupLegalEntityListResponseBody,
} from './api';
import { settlementMethodLabels } from './presentation';
import { fetchPublicationVocabulary, type PublicationVocabularyResponseBody } from './publication-draft-api';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import {
  Field,
  PathProblems,
  ReferencePicker,
  VocabularySelect,
  fieldLabel,
  momentPlaceholder,
  selectClass,
  useLoaded,
} from './PublicationFormFields';
import {
  contractChoiceKey,
  contractChoiceOf,
  emptySettlementPolicyDraft,
  methodCodesOf,
  settlementPolicyFieldPaths,
  settlementPolicyPayloadOf,
  withShellIntervalCopiedIntoPolicy,
  type ContractVersionChoice,
  type SettlementPolicyDraft,
} from './settlement-policy-form';

/**
 * 结算政策版本的逐字段表单（票 admin-write-faces/15；ADR-0101 决定八逐册裁形）。
 *
 * **为什么是逐字段表单**：低频、运营配置员操作、正文七格无子表——决定一判据（登记频次 × 操作者角色 × 载荷结构）
 * 的直接读数；本册正文不是矩阵，模板导入（决定二）只针对上百格的价卡。五步（预览摘要 → 存为待批准 → 批准 → 发布）由公共半边 PublicationDraftFlow 走，
 * 本组件只摆版本壳五格与政策正文，草稿 → 载荷在 settlement-policy-form.ts。
 *
 * **三样从读面选、一样从词表选、两样手填**：责任法人从集团法人册选，客户相对方从业务参与方册选（CONTEXT 结算方式
 * 那条规则末句，票 23 裁决一：相对方是承担结算责任的法律主体，不是货主客户账户——一个账户可以按相对方拥有多个结算账户，
 * 两者不能合并；票 15 从账户册选是照 seed 的写法，两读都通所以没锁死，现在锁死。`CounterpartyReference` 仍是未绑定册的
 * 引用，存在性不由构造门查，这里只换选单来源），合同从客户与合同目录选且**对象 + 版本两格一起落**
 * （不让操作者手拼版本号；两段式串只许领域拼），选出的合同对象同时镜像成壳引用 references.CUSTOMER_CONTRACT
 * （CONTEXT 结算方式那条规则末句，票 23 裁决二——没有它，被引合同未发布时的排序门对本册不成立；镜像在
 * settlementPolicyPayloadOf 做，不给操作者第二个合同格）；结算方式下拉只吃服务端词表读口（票 20）答的码 × 本页中文词表，
 * 表单不内置 PREPAID / TERMS，词表读不到就显占位、不自造码、不预选；费用范围与币种是引用串手填，币种收 ISO 代码串、
 * 存不存在由服务端构造门答。读面在接入渠道未配置那堵墙前（403）或读不到时退回手填，表单不因此变死。
 *
 * **表单不算摘要、不收也不送批准人、不裁任何门**（伞票 07 硬句）：连「必填」都不在本地拦——空字段、区间先后、方式集外、
 * 引用在不在册一律送上去，预览口逐格 problems 回来挂到对应格旁；本地没有任何一格编不进载荷类型（全是文本），所以
 * 不给 localProblems。**本册硬句**：结算政策答「怎么结」不答「要不要接受前控制」——这里没有、也不会长出控制字段。
 */
export interface SettlementPolicyPublicationFormProps {
  /** 载体到达「发布」那一步时回调，政策页借它切到结算政策册并重读。 */
  onPublished?: () => void;
}

const kind = 'SETTLEMENT_POLICY';

export function SettlementPolicyPublicationForm({ onPublished }: SettlementPolicyPublicationFormProps) {
  const [draft, setDraft] = useState<SettlementPolicyDraft>(emptySettlementPolicyDraft());
  const patch = (change: Partial<SettlementPolicyDraft>) => setDraft((current) => ({ ...current, ...change }));
  // 结算方式的封闭集只从服务端词表读口取（票 20，`kind=SETTLEMENT_POLICY` 的 `method` 一集）；本表单不内置 PREPAID / TERMS，
  // 内置一份就是同一封闭集的第二份写法（票 20 立票理由）。
  const vocabulary = useLoaded(loadSettlementVocabulary);
  const methodCodes = vocabulary?.kind === 'outcome' ? methodCodesOf(vocabulary.body.sets) : null;

  return (
    <PublicationDraftFlow
      kind="SETTLEMENT_POLICY"
      title="发布结算政策版本"
      assemblePayload={() => settlementPolicyPayloadOf(draft)}
      fieldPaths={settlementPolicyFieldPaths}
      onPublished={onPublished}
    >
      {({ problems, locked }: PublicationFormContext) => (
        <div className="flex flex-col gap-5">
          <section className="flex flex-col gap-3">
            <h3 className="text-[13px] font-medium text-idpxyz-text">版本壳</h3>
            <p className="text-[11px] text-idpxyz-textMuted">
              壳上的适用范围与区间是<strong>版本</strong>的（登记册逐列比对的项），与下面政策正文自己的适用区间是两样；
              区间上界留空即持续有效。壳上的指名引用不另填——正文六维里选出的合同同时作壳引用交出。
            </p>
            <div className="grid grid-cols-2 gap-3">
              <Field label="政策对象标识 *" path="objectId" problems={problems}>
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
            <div className="flex items-center justify-between">
              <h3 className="text-[13px] font-medium text-idpxyz-text">政策正文（0011）</h3>
              <Button variant="outline" disabled={locked} onClick={() => setDraft(withShellIntervalCopiedIntoPolicy)}>
                区间从版本壳带入
              </Button>
            </div>
            <p className="text-[11px] text-idpxyz-textMuted">
              结算政策答的是<strong>怎么结</strong>（预付 / 账期）在哪个精确范围适用：责任法人 × 客户相对方 × 合同版本 × 费用范围 ×
              币种 × 区间，六维一维不少，同一范围两法命中即适用冲突。它不答「要不要接受前控制」——那归客户合同的合同级声明与
              接受前财务控制策略两册，本表单没有控制字段。
            </p>
            <div className="grid grid-cols-2 gap-3">
              <VocabularySelect
                label="结算方式 *"
                path="settlementPolicy.method"
                setName="method"
                kind={kind}
                codes={methodCodes}
                vocabulary={vocabulary}
                labels={settlementMethodLabels}
                problems={problems}
                value={draft.method}
                locked={locked}
                onChange={(method) => patch({ method })}
                // 句中「枚举、」后那个半角空格是此前本表单里 JSX 折行留下的渲染结果，照原样保留——本票不改一字显示文案。
                unavailableNote="方式的码只从服务端词表读口取（GET /commercial-publication-vocabularies?kind=SETTLEMENT_POLICY），表单不内置枚举、 不自造码；词表就绪前这一格选不了，送预览会由服务端点名。"
              />
              <ReferencePicker
                label="责任法人（集团法人）*"
                path="settlementPolicy.legalEntity"
                problems={problems}
                value={draft.legalEntity}
                locked={locked}
                onChange={(legalEntity) => patch({ legalEntity })}
                load={listGroupLegalEntities}
                optionsOf={(body: GroupLegalEntityListResponseBody) =>
                  body.entities.map((entity) => ({
                    value: entity.legalEntityId,
                    label: `${entity.legalEntityId} · ${entity.partyNameKnown ? entity.partyName : entity.partyId} · ${entity.status}`,
                  }))
                }
                emptyNote="当前租户尚无集团法人；先在集团与法人册登记责任法人。"
                readFace="集团法人册"
              />
              <ReferencePicker
                label="客户相对方（业务参与方）*"
                path="settlementPolicy.counterparty"
                problems={problems}
                value={draft.counterparty}
                locked={locked}
                onChange={(counterparty) => patch({ counterparty })}
                load={listBusinessParties}
                optionsOf={(body: BusinessPartyListResponseBody) =>
                  body.parties.map((party) => ({
                    value: party.partyId,
                    label: `${party.partyId} · ${party.partyName} · ${party.status}`,
                  }))
                }
                emptyNote="当前租户尚无业务参与方；先在业务参与方册登记，再发布结算政策。"
                readFace="业务参与方册"
              />
              <ContractPicker
                problems={problems}
                objectId={draft.contractObjectId}
                version={draft.contractVersion}
                locked={locked}
                onChange={(choice) => patch({ contractObjectId: choice.objectId, contractVersion: choice.version })}
              />
              <Field label="费用范围 *" path="settlementPolicy.chargeScope" problems={problems}>
                <Input
                  value={draft.chargeScope}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder="服务 / 费用范围引用串"
                  onChange={(event) => patch({ chargeScope: event.target.value })}
                />
              </Field>
              <Field label="币种（ISO 代码）*" path="settlementPolicy.currency" problems={problems}>
                <Input
                  value={draft.currency}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder="如 CNY；存不存在由服务端答"
                  onChange={(event) => patch({ currency: event.target.value })}
                />
              </Field>
              <Field label="政策生效起点 *" path="settlementPolicy.effectiveStartsAt" problems={problems}>
                <Input
                  value={draft.policyEffectiveStartsAt}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder={momentPlaceholder}
                  onChange={(event) => patch({ policyEffectiveStartsAt: event.target.value })}
                />
              </Field>
              <Field label="政策生效止点（可空）" path="settlementPolicy.effectiveEndsAt" problems={problems}>
                <Input
                  value={draft.policyEffectiveEndsAt}
                  disabled={locked}
                  className="font-mono text-[13px]"
                  placeholder="留空即无上界"
                  onChange={(event) => patch({ policyEffectiveEndsAt: event.target.value })}
                />
              </Field>
            </div>
          </section>
        </div>
      )}
    </PublicationDraftFlow>
  );
}

// 稳定的函数引用：useLoaded 以它为依赖，写成内联箭头会每次渲染重取。
function loadSettlementVocabulary(): Promise<ApiResult<PublicationVocabularyResponseBody>> {
  return fetchPublicationVocabulary('SETTLEMENT_POLICY');
}

/**
 * 合同从客户与合同目录选，**对象 + 版本两格一起落**：每一版合同一项，选中即把两格同时写进草稿，操作者不手拼版本号
 * （两段式串只许领域 NewQualifiedVersionLabel 一处拼）。目录读不到时退回两个手填格——仍是两格，不是一格串。
 * 不按状态过滤（表单不裁，状态显在选项里由人看）；选出来的只是引用，在不在册、版本对不对仍由服务端判。
 * 这一格同时是壳引用 references.CUSTOMER_CONTRACT 的唯一来源（票 23 裁决二）：镜像在 settlementPolicyPayloadOf 做，
 * 这里不多一格；壳引用那条路径的问题也显在这一格下。
 */
function ContractPicker({
  problems,
  objectId,
  version,
  locked,
  onChange,
}: {
  problems: Record<string, string[]>;
  objectId: string;
  version: string;
  locked: boolean;
  onChange: (choice: ContractVersionChoice) => void;
}) {
  const answer = useLoaded(listCustomerContracts);
  const objectPath = 'settlementPolicy.contract.objectId';
  const versionPath = 'settlementPolicy.contract.version';
  // 壳上的指名引用是从这一格镜像出去的（settlementPolicyPayloadOf），没有第二个输入格；服务端核出壳引用与六维合同
  // 不一致时点名的是壳引用那条路径，那条问题只能显在这里——两种形态都带上它，认领表才不多出一条没处显的路径。
  const shellReferencePath = 'references.CUSTOMER_CONTRACT';
  const current: ContractVersionChoice = { objectId, version };
  const currentKey = objectId === '' && version === '' ? '' : contractChoiceKey(current);

  if (answer?.kind === 'outcome') {
    const contracts: ContractVersionChoice[] = answer.body.contracts.map((contract) => ({
      objectId: contract.objectId,
      version: contract.version,
    }));
    const known = contracts.some((contract) => contractChoiceKey(contract) === currentKey);
    return (
      <label className="block">
        <span className={fieldLabel}>
          客户合同版本（对象 + 版本一起选）* <span className="font-mono text-[10px]">settlementPolicy.contract</span>
        </span>
        <select
          className={selectClass}
          value={currentKey}
          disabled={locked}
          onChange={(event) => {
            const picked = event.target.value;
            const choice = contractChoiceOf(picked, contracts) ?? (picked === currentKey ? current : null);
            onChange(choice ?? { objectId: '', version: '' });
          }}
        >
          <option value="">未选</option>
          {!known && currentKey !== '' ? (
            <option value={currentKey}>
              {objectId} · {version}（手填，不在目录上）
            </option>
          ) : null}
          {answer.body.contracts.map((contract) => (
            <option key={contractChoiceKey(contract)} value={contractChoiceKey(contract)}>
              {contract.objectId} · {contract.version} · {contract.status} · {contract.scope}
            </option>
          ))}
        </select>
        {contracts.length === 0 ? (
          <span className="block text-[11px] text-idpxyz-textMuted mt-1">
            当前租户尚无客户合同版本；先发布客户合同，再发布挂在它上面的结算政策。
          </span>
        ) : null}
        <PathProblems problems={problems} paths={[objectPath, versionPath, shellReferencePath]} />
      </label>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <Field label="客户合同对象 *" path={objectPath} alsoPaths={[shellReferencePath]} problems={problems}>
        <Input
          value={objectId}
          disabled={locked}
          className="font-mono text-[13px]"
          placeholder="合同对象标识（目录不可用时手填）"
          onChange={(event) => onChange({ objectId: event.target.value, version })}
        />
      </Field>
      <Field label="客户合同版本号 *" path={versionPath} problems={problems}>
        <Input
          value={version}
          disabled={locked}
          className="font-mono text-[13px]"
          placeholder="版本号（不是「对象/版本」串）"
          onChange={(event) => onChange({ objectId, version: event.target.value })}
        />
      </Field>
      <span className="block text-[11px] text-idpxyz-textMuted">
        {answer === null
          ? '正在读客户与合同目录…'
          : answer.kind === 'unconfigured'
            ? '客户与合同目录读口在接入渠道未配置那堵墙前（403），先分两格手填；合同在不在册由服务端发布时判。'
            : '客户与合同目录读不到，先分两格手填；合同在不在册由服务端发布时判。'}
      </span>
    </div>
  );
}