#!/usr/bin/env python3
from pathlib import Path
import argparse,json,yaml
from jsonschema import Draft202012Validator,RefResolver
EXPECTED={
'CalculationPurpose':['QUOTE','ESTIMATED_COST','ACTUAL_COST','CUSTOMER_BILLING','SIMULATION','REPLAY','DISPUTE_REVIEW'],
'PriceRole':['CARRIER_TARIFF','BUY','INTERNAL','SELL','PUBLIC'],
'ChargeScope':['PACKAGE','SHIPMENT','ORDER','MANIFEST','CONTAINER','INVOICE','SETTLEMENT_PERIOD'],
'ChargeBasisType':['NONE','WEIGHT','VOLUME','PIECE_COUNT','DECLARED_VALUE','BASE_FREIGHT','SELECTED_CHARGES','TOTAL_BEFORE_DISCOUNT','TOTAL_AFTER_DISCOUNT','EXCESS_WEIGHT','EXCESS_DIMENSION','DAYS','LOOKUP_RESULT','CUSTOM_BASIS'],
'ChargeMethodType':['FIXED','PER_UNIT','PERCENTAGE','LOOKUP','TIERED_BANDED','TIERED_PROGRESSIVE','MINIMUM_ADJUSTMENT','MAXIMUM_CAP','MAX_OF','FORMULA_TEMPLATE','CUSTOM_FUNCTION'],
'RatingAggregationMode':['PER_PACKAGE','SHIPMENT_TOTAL','FIRST_CONTINUE_SHIPMENT','MASTER_PLUS_CHILD','HYBRID'],
'ChargeEffect':['ADD','DEDUCT'],'RoundingMode':['CEILING','FLOOR','HALF_UP','HALF_EVEN','UP','DOWN']}
def deref(doc,p):
 if '$ref' not in p:return p
 return doc['components']['parameters'][p['$ref'].split('/')[-1]]
def main():
 ap=argparse.ArgumentParser();ap.add_argument('openapi');ap.add_argument('examples');a=ap.parse_args()
 doc=yaml.safe_load(Path(a.openapi).read_text(encoding='utf-8')); ex=json.loads(Path(a.examples).read_text(encoding='utf-8')); errors=[]
 if doc.get('openapi')!='3.1.0':errors.append('openapi must be 3.1.0')
 if doc.get('info',{}).get('version')!='1.0.1':errors.append('version must be 1.0.1')
 ops=[]
 for path,item in doc.get('paths',{}).items():
  for method,op in item.items():
   if method not in {'get','post','put','patch','delete'}:continue
   if not op.get('operationId'):errors.append(f'{method} {path}: missing operationId')
   else:ops.append(op['operationId'])
   names={(deref(doc,p).get('in'),deref(doc,p).get('name')) for p in op.get('parameters',[])}
   if method=='post':
    if ('header','Idempotency-Key') not in names:errors.append(f'POST {path}: missing Idempotency-Key')
    if path.endswith(('/offer','/accept','/cancel','/repricings')) and ('header','If-Match') not in names:errors.append(f'POST {path}: missing If-Match')
   if '304' in op.get('responses',{}) and ('header','If-None-Match') not in names:errors.append(f'{method.upper()} {path}: 304 without If-None-Match')
   if method=='post' and any(c in op.get('responses',{}) for c in ('201','202')):
    for c in ('201','202'):
     if c in op.get('responses',{}) and '$ref' not in op['responses'][c] and 'Location' not in op['responses'][c].get('headers',{}):errors.append(f'POST {path} {c}: missing Location')
 if len(ops)!=len(set(ops)):errors.append('duplicate operationId')
 for name,expected in EXPECTED.items():
  if doc['components']['schemas'][name].get('enum')!=expected:errors.append(f'enum mismatch: {name}')
 resolver=RefResolver.from_schema(doc)
 for item in ex['examples']:
  v=Draft202012Validator({'$ref':f"#/components/schemas/{item['schema']}"},resolver=resolver)
  for e in v.iter_errors(item['instance']):errors.append(f"{item['name']} {list(e.path)}: {e.message}")
 for item in ex['examples']:
  if item['schema']=='RatingEvaluationCreateRequest':
   q=item['instance']
   if q.get('calculation_purpose')=='REPLAY':errors.append('REPLAY used on create endpoint')
   if q.get('calculation_purpose')=='ACTUAL_COST' and 'calculation_basis' not in q:errors.append('ACTUAL_COST missing basis')
   if q.get('pricing',{}).get('mode')=='EXPLICIT_COMPONENTS' and q.get('calculation_purpose')!='SIMULATION':errors.append('EXPLICIT_COMPONENTS not simulation')
 result={'status':'PASSED' if not errors else 'FAILED','openapi':doc.get('openapi'),'contract_version':doc.get('info',{}).get('version'),'path_count':len(doc.get('paths',{})),'operation_count':len(ops),'schema_count':len(doc.get('components',{}).get('schemas',{})),'example_count':len(ex['examples']),'errors':errors}
 print(json.dumps(result,ensure_ascii=False,indent=2));return 0 if not errors else 1
if __name__=='__main__':raise SystemExit(main())
