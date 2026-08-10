#!/usr/bin/env python3
import sys,yaml,json,re
from pathlib import Path
w=yaml.safe_load(Path(sys.argv[1]).read_text());s=yaml.safe_load(Path(sys.argv[2]).read_text());tr=json.loads(Path(sys.argv[3]).read_text());md=Path(sys.argv[4]).read_text(encoding='utf-8')
errors=[]; ids=[]; deps=[]
for e in w['epics']:
 for t in e['tasks']: ids.append(t['task_id']);deps += [(t['task_id'],d) for d in t['depends_on']]
if len(ids)!=len(set(ids)): errors.append('duplicate task id')
known=set(ids)
for a,d in deps:
 if d not in known: errors.append(f'unknown dependency {a}->{d}')
# cycle
G={i:[] for i in ids};ind={i:0 for i in ids}
for a,d in deps: G[d].append(a);ind[a]+=1
q=sorted([i for i,v in ind.items() if v==0]);seen=[]
while q:
 n=q.pop(0);seen.append(n)
 for x in sorted(G[n]):
  ind[x]-=1
  if ind[x]==0:q.append(x)
if len(seen)!=len(ids): errors.append('task dependency cycle')
if len(s['sprints'])!=12: errors.append('sprint count')
if len(tr['items'])!=len(ids): errors.append('traceability count')
if md.count('```')%2: errors.append('markdown fences')
for phrase in ['136 Golden Cases','Sprint 0','BUY/SELL','PostgreSQL','Compiled Plan','Definition of Done']:
 if phrase not in md: errors.append('missing '+phrase)
result={'status':'PASSED' if not errors else 'FAILED','epic_count':len(w['epics']),'task_count':len(ids),'sprint_count':len(s['sprints']),'traceability_count':len(tr['items']),'dependency_edges':len(deps),'errors':errors}
print(json.dumps(result,ensure_ascii=False,indent=2));sys.exit(0 if not errors else 1)
