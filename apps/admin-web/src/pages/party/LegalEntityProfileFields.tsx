import { useId } from 'react';
import { Button, Input, Textarea } from '@idpxyz/ui-primitives';
import type { ContactPurpose } from './api';
import { contactPurposeLabels, type ContactDraft, type LegalEntityProfileDraft } from './legal-entity-profile';
import { Field, Problems } from './PublicationFormFields';
import { FormGroup } from './party-registration-fields';

export function LegalEntityProfileFields({
  draft,
  patch,
  problems,
  locked,
}: {
  draft: LegalEntityProfileDraft;
  patch: (change: Partial<LegalEntityProfileDraft>) => void;
  problems: Record<string, string[]>;
  locked: boolean;
}) {
  const platformListId = useId();
  const setContact = (index: number, change: Partial<ContactDraft>) =>
    patch({ contacts: draft.contacts.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  return (
    <>
      <datalist id={platformListId}>
        {['微信', '企业微信', 'WhatsApp', 'Telegram', 'LINE', 'QQ', 'LinkedIn', 'Skype'].map((platform) => <option key={platform} value={platform} />)}
      </datalist>
      <FormGroup title="代表人 / 负责人" description="按登记资料填写；与业务联系人分别管理。">
        <Field label="身份类型" path="profiles[0].representative" problems={problems}>
          <select
            aria-label="代表人身份类型"
            value={draft.representativeKind}
            disabled={locked}
            onChange={(event) =>
              patch({ representativeKind: event.target.value as LegalEntityProfileDraft['representativeKind'] })
            }
            className="h-9 w-full rounded-md border border-idpxyz-border bg-idpxyz-card px-3 text-[13px] text-idpxyz-text"
          >
            <option value="">未配置</option>
            <option value="LEGAL_REPRESENTATIVE">法定代表人</option>
            <option value="RESPONSIBLE_PERSON">登记负责人</option>
          </select>
        </Field>
        <Field label="姓名" path="profiles[0].representative" problems={problems}>
          <Input
            aria-label="代表人姓名"
            value={draft.representativeName}
            readOnly={locked}
            onChange={(event) => patch({ representativeName: event.target.value })}
          />
        </Field>
      </FormGroup>
      <FormGroup title="地址" description="注册地址须与身份的注册国家 / 地区一致；办公和通信地址可分别登记。">
        {(
          [
            ['registeredAddress', '注册地址', 'addressCountry', 'addressLines'],
            ['officeAddress', '办公地址', 'officeCountry', 'officeLines'],
            ['correspondenceAddress', '通信地址', 'correspondenceCountry', 'correspondenceLines'],
          ] as const
        ).map(([path, label, country, lines]) => (
          <div key={path} className="md:col-span-2 grid gap-3 sm:grid-cols-[9rem_minmax(0,1fr)]">
            <Field
              label={`${label}国家 / 地区${path === 'registeredAddress' ? ' *' : ''}`}
              path={`profiles[0].${path}`}
              problems={problems}
            >
              <Input
                aria-label={`${label}国家 / 地区`}
                placeholder="如 CN、SG"
                value={draft[country]}
                readOnly={locked}
                onChange={(event) => patch({ [country]: event.target.value })}
              />
            </Field>
            <Field
              label={`${label}${path === 'registeredAddress' ? ' *' : '（选填）'}`}
              path={`profiles[0].${path}.lines`}
              problems={problems}
            >
              <Textarea
                aria-label={label}
                rows={2}
                value={draft[lines]}
                readOnly={locked}
                onChange={(event) => patch({ [lines]: event.target.value })}
              />
            </Field>
          </div>
        ))}
      </FormGroup>
      <FormGroup title="税务与开票" description="开票抬头留空表示此版本不带开票资料。">
        <Field label="开票抬头" path="profiles[0].invoiceTitle" problems={problems}>
          <Input
            aria-label="开票抬头"
            value={draft.invoiceTitle}
            readOnly={locked}
            onChange={(event) => patch({ invoiceTitle: event.target.value })}
          />
        </Field>
        <div className="md:col-span-2">
          {draft.taxNumbers.map((row, index) => (
            <div key={index} className="mb-3 grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-3">
              <Input
                aria-label={`税务登记号 ${index + 1} 类型`}
                placeholder="税务登记号类型码"
                value={row.typeCode}
                readOnly={locked}
                onChange={(event) =>
                  patch({
                    taxNumbers: draft.taxNumbers.map((item, at) =>
                      at === index ? { ...item, typeCode: event.target.value } : item,
                    ),
                  })
                }
              />
              <Input
                aria-label={`税务登记号 ${index + 1} 号码`}
                placeholder="税务登记号"
                value={row.number}
                readOnly={locked}
                onChange={(event) =>
                  patch({
                    taxNumbers: draft.taxNumbers.map((item, at) =>
                      at === index ? { ...item, number: event.target.value } : item,
                    ),
                  })
                }
              />
              <Button
                variant="ghost"
                disabled={locked}
                onClick={() => patch({ taxNumbers: draft.taxNumbers.filter((_, at) => at !== index) })}
              >
                移除
              </Button>
            </div>
          ))}
          <Problems lines={problems['profiles[0].taxRegistrationNumbers']} />
          <Button
            variant="outline"
            disabled={locked}
            onClick={() => patch({ taxNumbers: [...draft.taxNumbers, { typeCode: '', number: '' }] })}
          >
            添加税务登记号
          </Button>
        </div>
      </FormGroup>
      <FormGroup title="联系人" description="可配置多个用途；每种用途可分别指名一位主要联系人。">
        <div className="md:col-span-2 space-y-4">
          {draft.contacts.map((row, index) => (
            <fieldset key={index} disabled={locked} className="min-w-0 border-t border-idpxyz-border pt-3">
              <legend className="px-1 text-[13px] font-medium text-idpxyz-text">联系人 {index + 1}</legend>
              <div className="grid gap-3 sm:grid-cols-2">
                {(
                  [
                    ['name', '姓名 *'],
                    ['department', '部门'],
                    ['position', '职务'],
                    ['email', '邮箱'],
                    ['phone', '电话'],
                  ] as const
                ).map(([key, label]) => (
                  <Field
                    key={key}
                    label={label}
                    path={`profiles[0].contacts[${index}]${key === 'name' ? '' : '.' + key}`}
                    problems={problems}
                  >
                    <Input
                      aria-label={`联系人 ${index + 1} ${label}`}
                      value={row[key] ?? ''}
                      readOnly={locked}
                      onChange={(event) => setContact(index, { [key]: event.target.value })}
                    />
                  </Field>
                ))}
              </div>
              <div className="mt-4 space-y-2">
                <p className="text-[12px] font-medium text-idpxyz-text">社交账号 <span className="font-normal text-idpxyz-breadcrumb">（选填，可添加多个）</span></p>
                {(row.socialAccounts ?? []).map((social, accountIndex) => (
                  <div key={accountIndex} className="space-y-1">
                    <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto] items-start gap-2">
                      <Input
                        list={platformListId}
                        aria-label={`联系人 ${index + 1} 社交账号 ${accountIndex + 1} 平台`}
                        placeholder="选择或填写平台"
                        value={social.platform}
                        onChange={(event) => setContact(index, { socialAccounts: row.socialAccounts!.map((item, at) => at === accountIndex ? { ...item, platform: event.target.value } : item) })}
                      />
                      <Input
                        aria-label={`联系人 ${index + 1} 社交账号 ${accountIndex + 1} 账号`}
                        placeholder="账号或资料链接"
                        value={social.account}
                        onChange={(event) => setContact(index, { socialAccounts: row.socialAccounts!.map((item, at) => at === accountIndex ? { ...item, account: event.target.value } : item) })}
                      />
                      <Button variant="ghost" aria-label={`移除联系人 ${index + 1} 社交账号 ${accountIndex + 1}`} onClick={() => setContact(index, { socialAccounts: row.socialAccounts!.filter((_, at) => at !== accountIndex) })}>移除</Button>
                    </div>
                    <Problems lines={problems[`profiles[0].contacts[${index}].socialAccounts[${accountIndex}]`]} />
                  </div>
                ))}
                <Button variant="ghost" size="sm" aria-label={`为联系人 ${index + 1} 添加社交账号`} onClick={() => setContact(index, { socialAccounts: [...(row.socialAccounts ?? []), { platform: '', account: '' }] })}>添加社交账号</Button>
              </div>
              <div className="mt-3 flex flex-wrap gap-x-5 gap-y-2">
                {(Object.entries(contactPurposeLabels) as [ContactPurpose, string][]).map(([purpose, label]) => {
                  const selected = row.purposes?.includes(purpose) ?? false;
                  return (
                    <div key={purpose} className="flex flex-wrap items-center gap-2 text-[12px] text-idpxyz-text">
                      <label className="flex items-center gap-1.5">
                        <input
                          type="checkbox"
                          checked={selected}
                          onChange={(event) =>
                            setContact(index, {
                              purposes: event.target.checked
                                ? [...(row.purposes ?? []), purpose]
                                : (row.purposes ?? []).filter((value) => value !== purpose),
                              primaryFor: (row.primaryFor ?? []).filter((value) => value !== purpose),
                            })
                          }
                        />
                        {label}
                      </label>
                      {selected ? (
                        <label className="flex items-center gap-1 text-idpxyz-breadcrumb">
                          <input
                            type="checkbox"
                            aria-label={`联系人 ${index + 1} ${label}主要联系人`}
                            checked={row.primaryFor?.includes(purpose) ?? false}
                            onChange={(event) =>
                              patch({
                                contacts: draft.contacts.map((contact, at) => ({
                                  ...contact,
                                  primaryFor:
                                    at === index && event.target.checked
                                      ? [...(contact.primaryFor ?? []).filter((value) => value !== purpose), purpose]
                                      : (contact.primaryFor ?? []).filter((value) => value !== purpose),
                                })),
                              })
                            }
                          />
                          主要
                        </label>
                      ) : null}
                    </div>
                  );
                })}
              </div>
              <Button
                className="mt-2"
                variant="ghost"
                disabled={locked}
                onClick={() => patch({ contacts: draft.contacts.filter((_, at) => at !== index) })}
              >
                移除此联系人
              </Button>
            </fieldset>
          ))}
          <Button
            variant="outline"
            disabled={locked}
            onClick={() =>
              patch({
                contacts: [
                  ...draft.contacts,
                  { name: '', email: '', phone: '', department: '', position: '', purposes: [], primaryFor: [] },
                ],
              })
            }
          >
            添加联系人
          </Button>
        </div>
      </FormGroup>
    </>
  );
}
