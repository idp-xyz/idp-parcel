import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import {
  commercialRegistrationEndpoints,
  listBusinessParties,
  listCustomerAccounts,
  listGroupLegalEntities,
  partyIdentityOutcomeLabels,
  type BusinessPartyListResponseBody,
  type BusinessPartyRecord,
  type CustomerAccountListResponseBody,
  type GroupLegalEntityListResponseBody,
} from './api';
import { problemNote, registrationTitles } from './presentation';
import { Field, ReferencePicker, selectClass, useLoaded } from './PublicationFormFields';
import {
  RevisionField,
  SnapshotJsonDetails,
  WallTimeField,
  businessPartyPickerOptions,
  customerAccountPickerOptions,
  legalEntityPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
import {
  deactivationTargetsOf,
  emptyIdentityDeactivationDraft,
  identityDeactivationLocalProblems,
  identityDeactivationPayloadOf,
  identityKindOptions,
  suggestedDeactivationRevision,
  type IdentityDeactivationDraft,
  type IdentityRegisters,
} from './identity-deactivation-form';

/**
 * 身份停用的逐字段表单（票 admin-web-group-legal-entities/10 第 3 条）。停用口一个命令带种类，法人与客户账户的停用
 * 也走本签（页面上登记签那段头注说明为什么它摆在本页）；结果分别显示在集团与法人页、客户与合同页的
 * 「客户账户」签，本页读得见的是参与方身份那一册。壳在 useRegistrationForm，这里只摆本册的格。
 *
 * **本组件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：身份在不在册、已否停用、修订是否
 * 错位，一律送上去让服务端答；本地只拦编码层（修订号、停用时刻），纯函数在 identity-deactivation-form.ts。
 *
 * 标识按种类从对应的册上选：换种类即换 Picker（key 带种类），上一册的候选不留在下拉里。修订建议也按种类去对应册
 * 上数——参与方册由页面持有传进来，法人册与客户账户册在这里各读一次（Picker 自己另读一次：共享 Picker 不交出行、
 * 只交出候选，而建议要的是修订号；改它归收口票，本票不碰共享件）。
 */
export interface IdentityDeactivationFormProps {
  /** 页面已取回的参与方列表（种类为业务参与方时给修订号建议用）；没取到传 null。 */
  knownParties: readonly BusinessPartyRecord[] | null;
  /** 参与方列表的重取序号：每变一次，参与方那一册的候选重读一次。 */
  partiesVersion: number;
  /** 登记册答 `DEACTIVATED` 时回调，页面借它重取参与方列表让「已停用」+ 时点 + 依据立刻可见。 */
  onDeactivated: () => void;
}

const info = moduleInfoById['business-parties'];
const kind = 'identity-deactivation';
const endpoint = `POST ${commercialRegistrationEndpoints[kind]}`;
const kindOptions = identityKindOptions();

const pickerNotes = {
  emptyNote: '对应册今天为空；先登记身份，或手填标识由服务端判。',
  optionsNote: '候选不按状态过滤：已停用的再停一次由服务端答，表单不拦。',
} as const;

export function IdentityDeactivationForm({ knownParties, partiesVersion, onDeactivated }: IdentityDeactivationFormProps) {
  const legalEntities = useLoaded(listGroupLegalEntities);
  const accounts = useLoaded(listCustomerAccounts);
  const registers: IdentityRegisters = {
    parties: knownParties,
    legalEntities: legalEntities?.kind === 'outcome' ? legalEntities.body.entities : null,
    accounts: accounts?.kind === 'outcome' ? accounts.body.accounts : null,
  };

  const form = useRegistrationForm<IdentityDeactivationDraft>({
    kind,
    empty: emptyIdentityDeactivationDraft,
    suggestion: (draft) => suggestedDeactivationRevision(deactivationTargetsOf(draft.kind, registers), draft.id),
    localProblems: identityDeactivationLocalProblems,
    payloadOf: identityDeactivationPayloadOf,
    // 答 `DEACTIVATED`（PartyRegistryOutcome 里 PartyIdentityDeactivated 的线上名）才重取；`册上没有这一个身份`与
    // 修订错位都没写进去。JSON 镜像那条路上分不出种类（快照是未译的 JSON），一律重取参与方列表：多取一次是一个 GET，
    // 为分种类去解一份 unknown 不值。
    landedOutcome: 'DEACTIVATED',
    onLanded: () => onDeactivated(),
  });
  const { draft, patch, problems, locked, timeZone } = form;

  const targets = deactivationTargetsOf(draft.kind, registers);
  const known = targets?.find((row) => row.id === draft.id);
  const pickerProps = {
    label: '身份标识 *',
    path: 'deactivations[0].id',
    problems,
    value: draft.id,
    locked,
    onChange: (id: string) => patch({ id }),
    ...pickerNotes,
  };

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-xs text-idpxyz-textMuted">
            停用形成修订链上新的一笔，原修订不被改写；自停用时点起该身份不再支持新的商业决定。修订号是你声明自己看到的
            册面（册上最新 + 1），错位说明册面已被并发推进或意图已陈旧，由服务端拒而不是替你猜。关系不在这里停：关系的
            终止走撤销 / 到期 / 替代。提交打到 <span className="font-mono">{endpoint}</span>；租户不在表单上。
          </p>

          <div className="grid grid-cols-2 gap-3">
            <Field label="身份种类 *" path="deactivations[0].kind" problems={problems}>
              <select
                className={selectClass}
                value={draft.kind}
                disabled={locked}
                // 换种类连标识一起清：上一册的标识留在格里，会被送到另一册去查。
                onChange={(event) => patch({ kind: event.target.value, id: '' })}
              >
                <option value="">未选</option>
                {kindOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">
                法人与客户账户的停用也走本签；结果分别显示在集团与法人页、客户与合同页的「客户账户」签。
              </span>
            </Field>

            {draft.kind === 'BUSINESS_PARTY' ? (
              <ReferencePicker<BusinessPartyListResponseBody>
                key={`party-${partiesVersion}`}
                {...pickerProps}
                load={listBusinessParties}
                optionsOf={businessPartyPickerOptions}
                readFace="参与方册"
              />
            ) : draft.kind === 'LEGAL_ENTITY' ? (
              <ReferencePicker<GroupLegalEntityListResponseBody>
                key="legal-entity"
                {...pickerProps}
                load={listGroupLegalEntities}
                optionsOf={legalEntityPickerOptions}
                readFace="法人册"
              />
            ) : draft.kind === 'CUSTOMER_ACCOUNT' ? (
              <ReferencePicker<CustomerAccountListResponseBody>
                key="customer-account"
                {...pickerProps}
                load={listCustomerAccounts}
                optionsOf={customerAccountPickerOptions}
                readFace="客户账户册"
              />
            ) : (
              <Field label="身份标识 *" path="deactivations[0].id" problems={problems}>
                <Input
                  value={draft.id}
                  readOnly={locked}
                  className="font-mono text-[13px]"
                  placeholder="先选身份种类，再从对应册上选"
                  onChange={(event) => patch({ id: event.target.value })}
                />
              </Field>
            )}

            <RevisionField
              path="deactivations[0].revision"
              problems={problems}
              {...form.revisionField}
              note={
                known
                  ? `对应册里该身份最新修订 r${known.revision}，建议停用落点 r${known.revision + 1}；册面可能已陈旧，错位由服务端判。`
                  : '对应册里没有这一标识（或册没取到），建议为 1；停用一个不在册的身份会被服务端答「册上没有这一个身份」。'
              }
            />

            <Field label="停用依据 *" path="deactivations[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="停用依据引用"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>

            <WallTimeField
              label="停用时点 *"
              path="deactivations[0].at"
              problems={problems}
              value={draft.at}
              locked={locked}
              timeZone={timeZone}
              onChange={(at) => patch({ at })}
            />
          </div>

          <div className="flex items-start gap-3">
            <Button onClick={form.send} disabled={locked || !form.canSend}>
              {locked ? '提交中…' : '提交停用'}
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
