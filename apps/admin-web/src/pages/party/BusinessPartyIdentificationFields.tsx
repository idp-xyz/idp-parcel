import { useEffect } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import type { RegistrationNumberTypeListResponseBody } from './api';
import type { BusinessPartyDraft, PartyExternalReferenceRow, PartyNumberRow } from './business-party-form';
import { identityCountryOptions, identityTypeOptions } from './legal-entity-form';
import { externalReferencePurposeLabels } from './presentation';
import { Field, Problems, ReferencePickerFor, SearchableSelect, fieldLabel, useLoaded } from './PublicationFormFields';
import { listIdentityRegistrationNumberTypes, useRegistrationNumberTypeCandidates } from './registration-number-type-candidates';

const numberTypeCatalogueHint = '先到「基础数据 › 注册号类型」配置';

const purposeOptions = Object.entries(externalReferencePurposeLabels).map(([value, label]) => ({ value, label }));

/** 每一行在送出那一组里的路径前缀；空行不送，给它一个不会与任何答复对上的路径。 */
function sentRowPaths<Row>(rows: readonly Row[], filled: (row: Row) => boolean, key: string): string[] {
  let sent = 0;
  return rows.map((row, index) => (filled(row) ? `businessParties[0].${key}[${sent++}]` : `businessParties[0].${key}[unsent-${index}]`));
}

/**
 * 往来单位的识别信息几格（ADR-0367）：注册地与注册号、外部系统编码，新增表单与编辑态共用。全部选填。
 *
 * 不裁任何门（伞票 admin-write-faces/07 硬句）：国家与号缺一、号格式不合、号已被别家持有、改了号没带更正依据，一律送上去让
 * 服务端逐格答。只有一件是呈现判断：`correction` 给了才摆身份更正依据那一格——册上登过号、这一笔又改了或去掉它时，调用方
 * 才给（registeredNumbersChanged）。没改号就不摆，免得人顺手填了一条服务端会拒的更正依据；那一格收起时已填的值一并清掉，
 * 看不见的格不该随载荷送出去。
 *
 * 载荷不带空行（business-party-form.ts），服务端按送出的那一组点名第几行；这里每一行的路径按它在送出那一组里的位置算，
 * 空行不占位，问题才落在填了的那一行上。
 */
export function BusinessPartyIdentificationFields({
  draft,
  patch,
  problems,
  locked,
  correction,
  ourCompany = false,
  notice,
}: {
  draft: BusinessPartyDraft;
  patch: (change: Partial<BusinessPartyDraft>) => void;
  problems: Record<string, string[]>;
  locked: boolean;
  /** 给了就摆身份更正依据那一格；note 是那一格下的一句。 */
  correction?: { note: string };
  /** 我方公司的往来单位：不摆注册地与注册号，换成一句说明。 */
  ourCompany?: boolean;
  /** 识别信息区上方的一句提醒（新增表单撞上已在册编码时用）。 */
  notice?: string;
}) {
  const correcting = correction !== undefined;
  // 国家格读身份层类型那一问，每一行的类型格共用按注册地收窄的那一问（registration-number-type-candidates.ts）；还没选注册地时
  // 类型格交国家那一问，按空国家挑出零项，显「先选注册地」那句。
  const identityTypes = useLoaded(listIdentityRegistrationNumberTypes);
  const countryTypes = useRegistrationNumberTypeCandidates('IDENTITY', draft.registrationCountry);
  const typeCandidates = draft.registrationCountry.trim() === '' ? identityTypes : countryTypes;
  useEffect(() => {
    if (!correcting && draft.identityCorrectionBasis !== '') patch({ identityCorrectionBasis: '' });
  }, [correcting, draft.identityCorrectionBasis, patch]);
  const numberPaths = sentRowPaths(draft.lifetimeNumbers, (row) => row.typeCode !== '' || row.number !== '', 'lifetimeRegistrationNumbers');
  const referencePaths = sentRowPaths(
    draft.externalReferences,
    (row) => row.system !== '' || row.purpose !== '' || row.code !== '',
    'externalReferences',
  );
  const setNumber = (index: number, change: Partial<PartyNumberRow>) =>
    patch({ lifetimeNumbers: draft.lifetimeNumbers.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  const setReference = (index: number, change: Partial<PartyExternalReferenceRow>) =>
    patch({ externalReferences: draft.externalReferences.map((row, at) => (at === index ? { ...row, ...change } : row)) });

  return (
    <div className="flex flex-col gap-4">
      {notice ? (
        <p className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 type-body leading-5 text-idpxyz-text">{notice}</p>
      ) : null}
      {ourCompany ? (
        <p className="type-body text-idpxyz-breadcrumb">这是我方公司的往来单位：注册地与注册号登在公司主体上，这里不登。</p>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2">
            <ReferencePickerFor<RegistrationNumberTypeListResponseBody>
              answer={identityTypes}
              label="注册地（国家 / 地区）"
              path="businessParties[0].registrationCountry"
              problems={problems}
              value={draft.registrationCountry}
              locked={locked}
              onChange={(registrationCountry) => patch({ registrationCountry })}
              optionsOf={(body) => identityCountryOptions(body.registrationNumberTypes)}
              emptyNote={`还没有配置任何注册号类型：${numberTypeCatalogueHint}。`}
              readFace="注册号类型目录"
              manualPlaceholder="国家 / 地区码（如 CN、SG）"
              optionsNote="注册号的类型按注册地列出，先选它。"
            />
          </div>
    
          <div>
            <p className={fieldLabel}>注册号</p>
            <div className="flex flex-col gap-2">
              {draft.lifetimeNumbers.map((row, index) => (
                <div key={index} className="grid grid-cols-[1fr_1fr_auto] items-start gap-3">
                  <ReferencePickerFor<RegistrationNumberTypeListResponseBody>
                    answer={typeCandidates}
                    label={`类型（第 ${index + 1} 行）`}
                    path={`${numberPaths[index]}.typeCode`}
                    problems={problems}
                    value={row.typeCode}
                    locked={locked}
                    onChange={(typeCode) => setNumber(index, { typeCode })}
                    optionsOf={(body) => identityTypeOptions(body.registrationNumberTypes, draft.registrationCountry)}
                    emptyNote={`先选注册地；该国家 / 地区还没有配置注册号类型时，${numberTypeCatalogueHint}。`}
                    readFace="注册号类型目录"
                    manualPlaceholder="类型码（如 USCC、UEN）"
                  />
                  <Field label={`号（第 ${index + 1} 行）`} path={`${numberPaths[index]}.number`} problems={problems}>
                    <Input
                      value={row.number}
                      readOnly={locked}
                      className="font-mono type-value"
                      placeholder="注册号原样填"
                      aria-label={`注册号（第 ${index + 1} 行）`}
                      onChange={(event) => setNumber(index, { number: event.target.value })}
                    />
                  </Field>
                  <Button
                    variant="ghost"
                    className="mt-5 text-idpxyz-breadcrumb"
                    disabled={locked}
                    onClick={() => patch({ lifetimeNumbers: draft.lifetimeNumbers.filter((_, at) => at !== index) })}
                  >
                    移除
                  </Button>
                </div>
              ))}
            </div>
            <Problems lines={problems['businessParties[0].lifetimeRegistrationNumbers']} />
            <div className="mt-2 flex items-center gap-3">
              <Button
                variant="outline"
                className="shrink-0 whitespace-nowrap text-idpxyz-breadcrumb"
                disabled={locked}
                onClick={() => patch({ lifetimeNumbers: [...draft.lifetimeNumbers, { typeCode: '', number: '' }] })}
              >
                添加一个注册号
              </Button>
              <span className="type-body text-idpxyz-breadcrumb">同一个号在本租户只能登在一家单位上。</span>
            </div>
          </div>
        </>
      )}

      {correction ? (
        <Field label="身份更正依据 *" path="businessParties[0].identityCorrectionBasis" problems={problems}>
          <Input
            value={draft.identityCorrectionBasis}
            readOnly={locked}
            className="font-mono type-value"
            placeholder="说明当初录错的单据或审批号"
            aria-label="身份更正依据"
            onChange={(event) => patch({ identityCorrectionBasis: event.target.value })}
          />
          <span className="mt-1 block type-body text-idpxyz-breadcrumb">{correction.note}</span>
        </Field>
      ) : null}

      <div>
        <p className={fieldLabel}>外部系统编码</p>
        <div className="flex flex-col gap-2">
          {draft.externalReferences.map((row, index) => (
            <div key={index} className="grid grid-cols-[1fr_1fr_1fr_auto] items-start gap-3">
              <Field label={`系统（第 ${index + 1} 行）`} path={`${referencePaths[index]}.system`} problems={problems}>
                <Input
                  value={row.system}
                  readOnly={locked}
                  className="font-mono type-value"
                  placeholder="如 ERP"
                  aria-label={`外部系统（第 ${index + 1} 行）`}
                  onChange={(event) => setReference(index, { system: event.target.value })}
                />
              </Field>
              <Field label={`用途（第 ${index + 1} 行）`} path={`${referencePaths[index]}.purpose`} problems={problems}>
                <SearchableSelect
                  options={purposeOptions}
                  value={row.purpose}
                  disabled={locked}
                  onChange={(purpose) => setReference(index, { purpose })}
                />
              </Field>
              <Field label={`编码（第 ${index + 1} 行）`} path={`${referencePaths[index]}.code`} problems={problems}>
                <Input
                  value={row.code}
                  readOnly={locked}
                  className="font-mono type-value"
                  placeholder="对方系统里的编码"
                  aria-label={`外部编码（第 ${index + 1} 行）`}
                  onChange={(event) => setReference(index, { code: event.target.value })}
                />
              </Field>
              <Button
                variant="ghost"
                className="mt-5 text-idpxyz-breadcrumb"
                disabled={locked}
                onClick={() => patch({ externalReferences: draft.externalReferences.filter((_, at) => at !== index) })}
              >
                移除
              </Button>
            </div>
          ))}
        </div>
        <Problems lines={problems['businessParties[0].externalReferences']} />
        <div className="mt-2 flex items-center gap-3">
          <Button
            variant="outline"
            className="shrink-0 whitespace-nowrap text-idpxyz-breadcrumb"
            disabled={locked}
            onClick={() => patch({ externalReferences: [...draft.externalReferences, { system: '', purpose: '', code: '' }] })}
          >
            添加一个外部编码
          </Button>
          <span className="type-body text-idpxyz-breadcrumb">这家单位在财务或其他系统里的编码，同一系统同一用途只填一个。</span>
        </div>
      </div>
    </div>
  );
}
