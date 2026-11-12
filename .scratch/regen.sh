#!/bin/bash
set -e
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
REPO=/home/tops/workspace/idp-parcel-landing
cd "$REPO"

echo "== adr-index"
go run ./tools/adr-index
if ! git diff --quiet -- docs/adr/README.md; then
  git add docs/adr/README.md
  git commit -q -m 'docs(adr): 重生成索引——落地 declared-goods-lines 链（ADR-0373、0374、0376、0379、0381、0382、0383）' --trailer 'Co-authored-by: Cursor <cursoragent@cursor.com>'
  echo committed adr-index
fi
go run ./tools/adr-index -check && echo adr-index-check-ok

echo "== at-coverage-counts"
go run ./tools/at-coverage-counts
if ! git diff --quiet; then
  git add -A
  git commit -q -m 'docs(scratch): 重算 AT 盘点总览计数——declared-goods-lines 链入列' --trailer 'Co-authored-by: Cursor <cursoragent@cursor.com>'
  echo committed at-coverage
fi
go run ./tools/at-coverage-counts -check && echo at-coverage-check-ok

echo "== mechanism-inventory"
(cd tools/mechanism-inventory && go run . -dir "$REPO" -out "$REPO/docs/product/MECHANISM-INVENTORY.md")
if ! git diff --quiet -- docs/product/MECHANISM-INVENTORY.md; then
  git add docs/product/MECHANISM-INVENTORY.md
  git commit -q -m 'docs(product): 重生成机制盘点——落地 declared-goods-lines 链' --trailer 'Co-authored-by: Cursor <cursoragent@cursor.com>'
  echo committed mechanism-inventory
fi
test -z "$(git status --porcelain -- docs/product/MECHANISM-INVENTORY.md)" && echo mech-clean
git log --oneline -4
echo REGEN_DONE
