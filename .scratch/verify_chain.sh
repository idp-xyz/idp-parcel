#!/bin/bash
set -e
cd /home/tops/workspace/idp-parcel-landing
# 代码链文件：分支尖 vs 落地尖
git diff --name-only f55370512..feat/goods-details-admin > /tmp/chainfiles.txt
echo "== code chain files with diff vs landed tip:"
xargs -a /tmp/chainfiles.txt git diff --name-only feat/goods-details-admin HEAD -- | tee /tmp/codediff.txt
echo "== docs chain files with diff vs landed tip:"
git diff --name-only 8d6f5ffba..docs/declared-goods-lines > /tmp/docsfiles.txt
xargs -a /tmp/docsfiles.txt git diff --name-only docs/declared-goods-lines HEAD -- | tee /tmp/docsdiff.txt
echo DONE
