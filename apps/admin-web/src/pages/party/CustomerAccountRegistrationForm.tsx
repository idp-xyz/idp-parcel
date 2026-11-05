import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { partyIdentityOutcomeLabels, type BusinessPartyListResponseBody, type CustomerAccountRecord } from './api';
import { problemNote, registrationTitles } from './presentation';
import { Field, ReferencePickerFor } from './PublicationFormFields';
import {
  AdvancedGroup,
  RevisionField,
  SnapshotJsonDetails,
  WallTimeField,
  businessPartyPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
import { advancedGroupOpen } from './registration-form';
import {
  customerAccountLocalProblems,
  customerAccountPayloadOf,
  emptyCustomerAccountDraft,
  suggestedCustomerAccountRevision,
  type CustomerAccountDraft,
} from './customer-account-form';

/**
 * 「新增客户账户」表单（票 business-parties-object-page/03）：取代原来粘 JSON 快照的登记面。壳在 useRegistrationForm，这里只摆本册的格：
 * 账户编码、所属客户（从往来单位册选）、来源单据、生效时间；修订号由表单按已取回的账户册算好、收在「高级」里。
 *
 * 两处用：客户与合同页的「新增客户账户」登记视图（整页形态），往来单位对象页「关联」里客户账户块就地登记（`inline`，所属客户由
 * `preset` 锁成这家单位）。**不算摘要、不裁任何门、不判领域规则**——所属单位在不在册、是不是同租户，由服务端逐格答。
 */
export interface CustomerAccountRegistrationFormProps {
  /** 页面已取回的账户册（给修订号建议与「已在册」提示用）；没取到传 null，建议一律为 1。 */
  knownAccounts: readonly CustomerAccountRecord[] | null;
  /** 页面持有的往来单位列表答案（给所属客户的候选用）；首取回来之前为 null。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  /** 登记册答 `REGISTERED` 时回调，页面借它重取账户册。 */
  onRegistered: () => void;
  /** 草稿有没有没提交的输入；卸载时报 false。 */
  onUnsavedChange?: (unsaved: boolean) => void;
  /** 预填：对象页上所属客户固定是这家单位，格锁死。 */
  preset?: Partial<CustomerAccountDraft>;
  /** 嵌入形态：无卡片外壳与 JSON 导入，动作行多「取消」。 */
  inline?: { onCancel: () => void };
}

const info = moduleInfoById['party-contracts'];
const kind = 'customer-account';
const advancedPaths = ['customerAccounts[0].revision'] as const;

export function CustomerAccountRegistrationForm({
  knownAccounts,
  parties,
  onRegistered,
  onUnsavedChange,
  preset,
  inline,
}: CustomerAccountRegistrationFormProps) {
  const partyLocked = preset?.customerPartyId !== undefined;
  const form = useRegistrationForm<CustomerAccountDraft>({
    kind,
    empty: () => ({ ...emptyCustomerAccountDraft(), ...preset }),
    suggestion: (draft) => suggestedCustomerAccountRevision(knownAccounts, draft.accountId),
    localProblems: customerAccountLocalProblems,
    payloadOf: customerAccountPayloadOf,
    landedOutcome: 'REGISTERED',
    onLanded: onRegistered,
  });
  const { draft, patch, problems, locked, timeZone, unsaved } = form;
  useEffect(() => {
    onUnsavedChange?.(unsaved);
  }, [onUnsavedChange, unsaved]);
  useEffect(() => () => onUnsavedChange?.(false), [onUnsavedChange]);

  // 按原串找而不裁空白：载荷也不裁，" X" 送上去就是另一个账户。
  const known = knownAccounts?.find((row) => row.accountId === draft.accountId);
  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);
  const advancedOpen = advancedGroupOpen(advancedChoice, problems, advancedPaths, known !== undefined);
  const advancedSummary = form.revisionField.edited
    ? `更新序号改为 ${form.revisionField.value}`
    : known
      ? `这是账户 ${known.accountId} 的第 ${form.revisionField.suggestion} 次更新，序号已算好`
      : '首次登记，序号由系统填';

  const body = (
    <>
      <p className="type-body text-idpxyz-breadcrumb">
        客户账户是客户在我们这里下单、结算用的账号，钉在一家往来单位上。要更正已登记的账户，填同一个账户编码再保存一次，历次记录都会保留。
      </p>
      <div className="grid grid-cols-2 gap-3">
        <Field label="账户编码 *" path="customerAccounts[0].accountId" problems={problems}>
          <Input
            value={draft.accountId}
            readOnly={locked}
            className="font-mono type-value"
            placeholder="如 ACCOUNT-01"
            onChange={(event) => patch({ accountId: event.target.value })}
          />
          <span className="mt-1 block type-body text-idpxyz-breadcrumb">
            {known ? `这个编码已登记在 ${known.customerPartyId} 名下；这次保存会更新它。` : '登记后不能改，前后不要带空格。'}
          </span>
        </Field>
        <ReferencePickerFor<BusinessPartyListResponseBody>
          label="所属客户 *"
          path="customerAccounts[0].customerPartyId"
          problems={problems}
          value={draft.customerPartyId}
          locked={locked || partyLocked}
          onChange={(customerPartyId) => patch({ customerPartyId })}
          answer={parties}
          optionsOf={businessPartyPickerOptions}
          emptyNote="还没有登记任何往来单位；先在「往来单位」里登记客户，或直接填编码。"
          readFace="往来单位"
          optionsNote="账户归哪家客户单位；同一家可以有多个账户。"
        />
        <Field label="来源单据 *" path="customerAccounts[0].basis" problems={problems}>
          <Input
            value={draft.basis}
            readOnly={locked}
            className="font-mono type-value"
            placeholder="开户申请、合同编号或审批单号"
            onChange={(event) => patch({ basis: event.target.value })}
          />
        </Field>
        <WallTimeField
          label="生效时间 *"
          path="customerAccounts[0].effectiveFrom"
          problems={problems}
          value={draft.effectiveFrom}
          locked={locked}
          timeZone={timeZone}
          onChange={(effectiveFrom) => patch({ effectiveFrom })}
          note="从这一刻起这个账户可以下单；填未来的时间就先显示为待生效。"
        />
      </div>
      <AdvancedGroup open={advancedOpen} onToggle={() => setAdvancedChoice(!advancedOpen)} summary={advancedSummary}>
        <RevisionField
          label="更新序号"
          path="customerAccounts[0].revision"
          problems={problems}
          {...form.revisionField}
          note="系统按已登记的记录自动算好，一般不用改。保存时提示序号对不上，说明列表已过时，刷新后再试。"
        />
      </AdvancedGroup>
    </>
  );
  const answer = (
    <RegistrationAnswerNote state={form.state} owner={info.owner} outcomeLabels={partyIdentityOutcomeLabels} problemNote={problemNote} />
  );
  if (inline) {
    return (
      <div className="flex flex-col gap-4">
        {body}
        <div className="flex flex-wrap items-start justify-between gap-3 border-t border-idpxyz-border pt-3">
          <div className="min-w-0 flex-1 type-body">{answer}</div>
          <div className="flex shrink-0 items-center gap-2">
            <Button variant="outline" size="sm" className="type-body" disabled={locked} onClick={inline.onCancel}>
              取消
            </Button>
            <Button variant="default" size="sm" className={`${primaryActionClass} type-body`} onClick={form.send} disabled={locked || !form.canSend}>
              {locked ? '保存中…' : '保存'}
            </Button>
          </div>
        </div>
      </div>
    );
  }
  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {body}
          <div className="flex items-start gap-3">
            <Button variant="default" className={primaryActionClass} onClick={form.send} disabled={locked || !form.canSend}>
              {locked ? '保存中…' : '保存'}
            </Button>
            {answer}
          </div>
        </CardContent>
      </Card>
      <SnapshotJsonDetails kind={kind} submit={form.submit} />
    </div>
  );
}
