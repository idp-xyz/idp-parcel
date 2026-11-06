import { useMemo } from 'react';
import { listRegistrationNumberTypes, type BusinessPartyIdentificationRecord } from './api';
import { partyIdentificationCellsOf, type PartyIdentificationCells } from './business-party-identification';
import { registrationNumberTypeNamesOf } from './legal-entity-identity';
import { countryLabel } from '../refdata-names';
import { CountryLabel } from '../refdata-labels';
import { RefText } from './copy-affordance';
import { DetailRow, Fact } from './detail-primitives';
import { useLoaded } from './PublicationFormFields';

/**
 * 识别信息的读出格，类型码经注册号类型目录译成目录译名；目录读不到就按码显。目录走取数层的共享缓存（useLoaded），
 * 概要与右栏各调一次也只读一遍。
 */
function usePartyIdentificationCells(record: BusinessPartyIdentificationRecord): PartyIdentificationCells {
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);
  const typeNameOf = useMemo(
    () => registrationNumberTypeNamesOf(typeCatalogue?.kind === 'outcome' ? typeCatalogue.body.registrationNumberTypes : []),
    [typeCatalogue],
  );
  return partyIdentificationCellsOf(record, typeNameOf);
}

/**
 * 右栏头区 meta 行上的识别信息（照公司主体右栏的 meta 行）：注册地、每个注册号、每个外部编码各一格，号与码点击即复制。
 * 没登的格不出现——往来单位的识别信息本来可缺，头区不为空格占位；要看「未登记」去对象页概要。
 */
export function BusinessPartyIdentificationFacts({ record }: { record: BusinessPartyIdentificationRecord }) {
  const cells = usePartyIdentificationCells(record);
  return (
    <>
      {cells.country !== null ? (
        <Fact inline label="注册地">
          <CountryLabel code={cells.country} />
        </Fact>
      ) : null}
      {cells.numbers.map((cell, index) => (
        <Fact inline label="注册号" key={`number-${cell.number}#${index}`}>
          <span className="text-idpxyz-breadcrumb">{cell.typeName}</span> <RefText value={cell.number} name={cell.typeName} />
        </Fact>
      ))}
      {cells.references.map((cell) => (
        <Fact inline label={cell.purpose} key={`reference-${cell.system}-${cell.purpose}`}>
          <span className="text-idpxyz-breadcrumb">{cell.system}</span> <RefText value={cell.code} name={`${cell.system} ${cell.purpose}`} />
        </Fact>
      ))}
    </>
  );
}

/**
 * 往来单位识别信息的三行读出（ADR-0367），对象页概要用。没登的格显「未登记」：识别信息本来可缺，空着不是错。
 */
export function BusinessPartyIdentificationRows({ record }: { record: BusinessPartyIdentificationRecord }) {
  const cells = usePartyIdentificationCells(record);
  const unregistered = <span className="text-idpxyz-breadcrumb">未登记</span>;
  return (
    <>
      <DetailRow label="注册地">{cells.country === null ? unregistered : countryLabel(cells.country)}</DetailRow>
      <DetailRow label="注册号">
        {cells.numbers.length === 0 ? (
          unregistered
        ) : (
          <span className="flex flex-col gap-0.5">
            {cells.numbers.map((cell) => (
              <span key={`${cell.typeName}-${cell.number}`}>
                <span className="text-idpxyz-breadcrumb">{cell.typeName}</span> <span className="font-mono">{cell.number}</span>
              </span>
            ))}
          </span>
        )}
      </DetailRow>
      <DetailRow label="外部系统编码">
        {cells.references.length === 0 ? (
          unregistered
        ) : (
          <span className="flex flex-col gap-0.5">
            {cells.references.map((cell) => (
              <span key={`${cell.system}-${cell.purpose}`}>
                <span className="font-mono">{cell.system}</span> <span className="text-idpxyz-breadcrumb">{cell.purpose}</span>{' '}
                <span className="font-mono">{cell.code}</span>
              </span>
            ))}
          </span>
        )}
      </DetailRow>
    </>
  );
}
