import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import { partyIdentityOutcomeLabels, type BusinessPartyRecord } from './api';
import { problemNote, registrationTitles } from './presentation';
import { Field } from './PublicationFormFields';
import { AdvancedGroup, FormGroup, RevisionField, SnapshotJsonDetails, WallTimeField, useRegistrationForm } from './party-registration-fields';
import { advancedGroupOpen } from './registration-form';
import {
  businessPartyLocalProblems,
  businessPartyPayloadOf,
  emptyBusinessPartyDraft,
  identificationChanged,
  registeredNumbersChanged,
  suggestedBusinessPartyRevision,
  type BusinessPartyDraft,
} from './business-party-form';
import { BusinessPartyIdentificationFields } from './BusinessPartyIdentificationFields';

/**
 * 「新增往来单位」表单（票 admin-web-group-legal-entities/10 第 1 条；ADR-0101 决定八自裁：格少、低频、无矩阵，直接逐字段表单）。
 * 壳在 useRegistrationForm，这里只摆本册的格。
 *
 * 表单只问业务要填的：名称、编码、来源单据、生效时间。修订号由表单按已取回的册算好、收在「高级」里（有问题时自动展开），
 * 不再当成一个要人填的必填项——运营不该自己数「第几次更新」。
 *
 * **本组件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：编码与名称是否为空、修订号连不连续，
 * 一律原样送上去让服务端逐格答，答回来的问题码经 field-problem-text.ts 译成「哪里错、怎么改」；本地只拦编码层（修订号编
 * 不进正整数、生效时刻换不成 RFC 3339），纯函数在 business-party-form.ts。**不带租户格**、**不裁首尾空白**，理由在那个文件头上。
 */
export interface BusinessPartyRegistrationFormProps {
  /** 页面已取回的参与方列表（给修订号建议用）；列表没取到传 null，建议一律为 1。 */
  knownParties: readonly BusinessPartyRecord[] | null;
  /**
   * 登记册答 `REGISTERED` 时回调，页面借它重取列表让新行在「往来单位」册立刻可见。逐字段那条路把送出的草稿交回，
   * 页面据它选中刚登的那一行（票 business-party-ux/06）；JSON 镜像那条路交 null。
   */
  onRegistered: (sent: BusinessPartyDraft | null) => void;
  /** 草稿有没有没提交的输入；页面据此决定离开登记视图前要不要先确认。卸载时报 false。 */
  onUnsavedChange?: (unsaved: boolean) => void;
}

const info = moduleInfoById['business-parties'];
const kind = 'business-party';
const advancedPaths = ['businessParties[0].revision'] as const;

export function BusinessPartyRegistrationForm({
  knownParties,
  onRegistered,
  onUnsavedChange,
}: BusinessPartyRegistrationFormProps) {
  const form = useRegistrationForm<BusinessPartyDraft>({
    kind,
    empty: emptyBusinessPartyDraft,
    suggestion: (draft) => suggestedBusinessPartyRevision(knownParties, draft.partyId),
    localProblems: businessPartyLocalProblems,
    payloadOf: businessPartyPayloadOf,
    // 答 `REGISTERED`（PartyRegistryOutcome 里 PartyIdentityRegistered 的线上名）才重取列表。
    landedOutcome: 'REGISTERED',
    onLanded: onRegistered,
  });
  const { draft, patch, problems, locked, timeZone, unsaved } = form;
  useEffect(() => {
    onUnsavedChange?.(unsaved);
  }, [onUnsavedChange, unsaved]);
  useEffect(() => () => onUnsavedChange?.(false), [onUnsavedChange]);

  // 按原串找而不裁空白：载荷也不裁，" X" 送上去就是另一个身份，这里若裁了就会把它说成「已在册」。
  const known = knownParties?.find((row) => row.partyId === draft.partyId);
  // 高级组：null 即操作者还没点过，按 advancedGroupOpen 的缺省走（编码已在册时展开——这一笔是更新，让人看见序号）。
  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);
  const advancedOpen = advancedGroupOpen(advancedChoice, problems, advancedPaths, known !== undefined);
  const advancedSummary = form.revisionField.edited
    ? `更新序号改为 ${form.revisionField.value}`
    : known
      ? `这是「${known.partyName}」的第 ${form.revisionField.suggestion} 次更新，序号已算好`
      : '首次登记，序号由系统填';

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {/* 字段的来历都说在这一段里，格下只留会改变操作的那一句（票 business-party-ux/06）——每格下面都挂一两句解释，整张表读起来像文档。 */}
          <p className="type-body text-idpxyz-breadcrumb">
            登记一家新的往来单位：名称、内部编码（建议用单位简称）、这次登记依据的单据，以及从何时起可以与它建立合作关系。要更正已登记单位的名称或
            识别信息，到它的对象页用「更正登记信息」，那里会带上它已登的内容。
          </p>

          <FormGroup title="单位身份">
            <Field label="名称 *" path="businessParties[0].name" problems={problems}>
              <Input
                value={draft.name}
                readOnly={locked}
                className="type-value"
                placeholder="如 深圳某某跨境电商有限公司"
                onChange={(event) => patch({ name: event.target.value })}
              />
            </Field>

            <Field label="编码 *" path="businessParties[0].partyId" problems={problems}>
              <Input
                value={draft.partyId}
                readOnly={locked}
                className="font-mono type-value"
                placeholder="如 SHIPPER-01"
                onChange={(event) => patch({ partyId: event.target.value })}
              />
              <span className="block type-body text-idpxyz-breadcrumb mt-1">
                {known ? `这个编码已登记为「${known.partyName}」；这次保存会更新它的名称或来源单据。` : '登记后不能改；前后不要带空格。'}
              </span>
            </Field>

            <Field label="来源单据 *" path="businessParties[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono type-value"
                placeholder="合同编号、邮件或审批单号"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>

            <WallTimeField
              label="生效时间 *"
              path="businessParties[0].effectiveFrom"
              problems={problems}
              value={draft.effectiveFrom}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveFrom) => patch({ effectiveFrom })}
              note="填未来的时间就先显示为待生效。"
            />
          </FormGroup>

          <FormGroup
            title="识别信息（选填）"
            description="注册号用来认出同一家公司有没有被登过两次；外部系统编码用来与财务等系统对上。"
          >
            <BusinessPartyIdentificationFields
              draft={draft}
              patch={patch}
              problems={problems}
              locked={locked}
              notice={
                known && identificationChanged(known, draft)
                  ? '这个编码已登记过，它已登的注册号或外部编码与本表单不同：保存后以本表单为准，没填的就去掉了。'
                  : undefined
              }
              correction={
                known && registeredNumbersChanged(known, draft)
                  ? { note: '这个编码已登过注册号，这次没有原样带上：要保留就在对象页用「更正登记信息」；是当初录错了，才在这里填更正依据。' }
                  : undefined
              }
            />
          </FormGroup>

          <AdvancedGroup open={advancedOpen} onToggle={() => setAdvancedChoice(!advancedOpen)} summary={advancedSummary}>
            <RevisionField
              label="更新序号"
              path="businessParties[0].revision"
              problems={problems}
              {...form.revisionField}
              note="系统按已登记的记录自动算好，一般不用改。保存时提示序号对不上，说明列表已过时，刷新后再试。"
            />
          </AdvancedGroup>

          <div className="flex items-start gap-3">
            <Button
              variant="default"
              className={primaryActionClass}
              onClick={form.send}
              disabled={locked || !form.canSend}
            >
              {locked ? '保存中…' : '保存'}
            </Button>
            <RegistrationAnswerNote
              state={form.state}
              owner={info.owner}
              outcomeLabels={partyIdentityOutcomeLabels}
              problemNote={problemNote}
            />
          </div>
        </CardContent>
      </Card>

      <SnapshotJsonDetails kind={kind} submit={form.submit} />
    </div>
  );
}
