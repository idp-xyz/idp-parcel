#!/usr/bin/env bash
# 替一个合成操作者向演示发行方要一枚令牌（票 operator-channel/17）。客户端与流程和管理台登录门相同：公共客户端
# idp-parcel-admin-web，authorization_code + PKCE（S256）。浏览器里那三步——跳授权端点、交登录表单、带 code
# 跳回回调地址——由本脚本走；回调地址只用来认出跳回的那一下，不真去访问，所以管理台不必在跑。
#
# 用法（Dex 已按 compose.yaml 起好）：
#   scripts/demo-seeds/operator-channel/operator-token.sh <email> [access_token|id_token|claims]
#
# 缺省打印访问令牌原文：管理台拿它作 Bearer（oidc.ts 的 accessTokenForRequest）。claims 打印两枚令牌的载荷，
# 只解码、不验签——验签是 parcel-api 的事，这里只供读 iss、sub 与 aud。只属隔离演示环境：email 与口令是
# dex.yaml 里的合成值。
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  printf '用法：%s <email> [access_token|id_token|claims]\n' "$0" >&2
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  printf 'operator-token: 需要 python3\n' >&2
  exit 1
fi

python3 - "$@" <<'PY'
import base64
import hashlib
import html.parser
import http.cookiejar
import json
import secrets
import sys
import urllib.error
import urllib.parse
import urllib.request

# 四个值都抄自 dex.yaml（发行方、客户端、回调、合成口令）与 oidc.ts（SCOPE）；那两份是权威，这里跟着改。
ISSUER = "http://127.0.0.1:5556/dex"
CLIENT_ID = "idp-parcel-admin-web"
REDIRECT_URI = "http://localhost:5173/auth/callback"
PASSWORD = "syn-demo-password"
SCOPE = "openid profile email offline_access"

email = sys.argv[1]
want = sys.argv[2] if len(sys.argv) > 2 else "access_token"
if want not in ("access_token", "id_token", "claims"):
    sys.exit(f"operator-token: 第二个参数只认 access_token、id_token、claims，收到 {want!r}")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    # 跳转逐跳自己走：跳向回调地址的那一下要截下来读 code，不能让 urllib 替我们去访问它。
    def redirect_request(self, *args, **kwargs):
        return None


# ProxyHandler({})：发行方在回环上，宿主环境里若设了 HTTP(S)_PROXY，不能让请求绕去代理。
opener = urllib.request.build_opener(
    urllib.request.ProxyHandler({}),
    urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
    NoRedirect,
)


def fetch(url, form=None):
    data = urllib.parse.urlencode(form).encode() if form is not None else None
    try:
        response = opener.open(urllib.request.Request(url, data=data), timeout=10)
    except urllib.error.HTTPError as answered:
        response = answered
    except urllib.error.URLError as unreachable:
        sys.exit(f"operator-token: 连不上 {url}：{unreachable.reason}（演示发行方起了没有？）")
    return response.code, response.headers, response.read().decode("utf-8", "replace")


def follow(url, form=None):
    """沿发行方内部的跳转走，直到落在一张页面上，或跳向回调地址（交回那个地址、页面为 None）。"""
    for _ in range(10):
        status, headers, body = fetch(url, form)
        form = None
        if status in (301, 302, 303, 307, 308):
            target = urllib.parse.urljoin(url, headers["Location"])
            if target.startswith(REDIRECT_URI):
                return target, None
            url = target
            continue
        if status == 200:
            return url, body
        if status == 401:
            sys.exit("operator-token: 发行方不认这组 email 与口令（HTTP 401）：email 是否在 dex.yaml 的 staticPasswords 里？")
        sys.exit(f"operator-token: {url} 答 HTTP {status}：\n{body[:800]}")
    sys.exit("operator-token: 跳转超过十次仍未落定")


class LoginForm(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.action = None
        self.fields = {}
        self._inside = False

    def handle_starttag(self, tag, attrs):
        attributes = dict(attrs)
        if tag == "form" and self.action is None:
            self.action = attributes.get("action") or ""
            self._inside = True
        elif tag == "input" and self._inside and attributes.get("name"):
            if (attributes.get("type") or "text").lower() == "hidden":
                self.fields[attributes["name"]] = attributes.get("value") or ""

    def handle_endtag(self, tag):
        if tag == "form":
            self._inside = False


def payload(token):
    segment = token.split(".")[1]
    return json.loads(base64.urlsafe_b64decode(segment + "=" * (-len(segment) % 4)))


status, _, body = fetch(ISSUER + "/.well-known/openid-configuration")
if status != 200:
    sys.exit(f"operator-token: 发现文档答 HTTP {status}")
discovery = json.loads(body)
if discovery.get("issuer") != ISSUER:
    sys.exit(f"operator-token: 发现文档的 issuer 是 {discovery.get('issuer')!r}，不是 {ISSUER!r}")

verifier = secrets.token_urlsafe(48)
challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b"=").decode()
state = secrets.token_urlsafe(16)
nonce = secrets.token_urlsafe(16)
authorize = discovery["authorization_endpoint"] + "?" + urllib.parse.urlencode({
    "client_id": CLIENT_ID,
    "response_type": "code",
    "scope": SCOPE,
    "redirect_uri": REDIRECT_URI,
    "state": state,
    "nonce": nonce,
    "code_challenge": challenge,
    "code_challenge_method": "S256",
})

page_url, page = follow(authorize)
if page is None:
    sys.exit("operator-token: 没交登录表单就跳回了回调地址，发行方那边像是带着会话")
form = LoginForm()
form.feed(page)
if form.action is None:
    sys.exit(f"operator-token: {page_url} 上找不到登录表单")
fields = dict(form.fields, login=email, password=PASSWORD)
callback, page = follow(urllib.parse.urljoin(page_url, form.action), fields)
if page is not None:
    sys.exit(f"operator-token: 交了登录表单，发行方落在一张页面上（{callback}）而没有跳回回调地址")

answer = urllib.parse.parse_qs(urllib.parse.urlsplit(callback).query)
if "error" in answer:
    sys.exit(f"operator-token: 发行方拒绝授权：{answer['error'][0]}（{answer.get('error_description', [''])[0]}）")
if answer.get("state") != [state]:
    sys.exit("operator-token: 跳回的 state 与发起时不符")

status, _, body = fetch(discovery["token_endpoint"], {
    "grant_type": "authorization_code",
    "code": answer["code"][0],
    "redirect_uri": REDIRECT_URI,
    "client_id": CLIENT_ID,
    "code_verifier": verifier,
})
if status != 200:
    sys.exit(f"operator-token: 令牌端点答 HTTP {status}：{body}")
tokens = json.loads(body)
if not tokens.get("access_token") or not tokens.get("id_token"):
    sys.exit(f"operator-token: 令牌端点没给齐访问令牌与 ID 令牌：{sorted(tokens)}")
if payload(tokens["id_token"]).get("nonce") != nonce:
    sys.exit("operator-token: ID 令牌的 nonce 与发起时不符")

if want == "claims":
    print(json.dumps({
        "access_token": payload(tokens["access_token"]),
        "id_token": payload(tokens["id_token"]),
    }, ensure_ascii=False, indent=2))
else:
    print(tokens[want])
PY
