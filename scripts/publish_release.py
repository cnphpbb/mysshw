#!/usr/bin/env python3
"""发布 GitHub Release (手工流程, 不依赖 GitHub Actions)。

用法:
    # 1. 构建产物
    MYSISHW_VERSION=v26.10.25.0 mage pack

    # 2. 发布 (token 需含 repo scope, 或 fine-grained 含 Contents: write)
    python3 scripts/publish_release.py v26.10.25.0

    # 或指定 token
    GITHUB_TOKEN=xxx python3 scripts/publish_release.py v26.10.25.0

行为:
    - 从 changelog.<tag>.md 生成 release body (去掉首行 H1)
    - 追加 '## checksum files' 段 (运行时计算 sha1, 与上传产物一致)
    - 创建 release (已存在则跳过, 幂等)
    - 上传 dist/ 下所有 .tar.gz / .zip + checksum.txt

依赖:
    - token 来源优先级: $GITHUB_TOKEN > ~/.hermes/.env 的 GITHUB_TOKEN
    - 权限: 经典 PAT 需 repo scope; fine-grained PAT 需 Contents: Read and write
"""

import glob
import hashlib
import json
import os
import sys
import urllib.error
import urllib.request

API = "https://api.github.com"


def die(msg, code=1):
    print(f"error: {msg}", file=sys.stderr)
    sys.exit(code)


def get_token():
    tok = os.environ.get("GITHUB_TOKEN", "").strip()
    if tok:
        return tok
    env_path = os.path.expanduser("~/.hermes/.env")
    if os.path.isfile(env_path):
        with open(env_path, encoding="utf-8", errors="ignore") as f:
            for line in f:
                if line.startswith("GITHUB_TOKEN="):
                    tok = line.split("=", 1)[1].strip().strip('"').strip("'")
                    if tok:
                        return tok
    die("找不到 GITHUB_TOKEN (设置环境变量, 或写入 ~/.hermes/.env)")


def api(method, path, token, payload=None, raw=None, content_type=None):
    url = path if path.startswith("http") else API + path
    headers = {
        "Authorization": f"token {token}",
        "Accept": "application/vnd.github+json",
        "User-Agent": "mysshw-publish-release",
    }
    data = None
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
    elif raw is not None:
        data = raw
        headers["Content-Type"] = content_type or "application/octet-stream"

    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req) as resp:
            body = resp.read()
            return resp.status, body
    except urllib.error.HTTPError as e:
        return e.code, e.read()
    except urllib.error.URLError as e:
        die(f"网络错误: {e}")


def sha1(path):
    h = hashlib.sha1()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def build_body(tag, dist):
    chg = f"changelog.{tag}.md"
    if not os.path.isfile(chg):
        die(f"找不到 {chg}")

    lines = open(chg, encoding="utf-8").read().split("\n")
    if lines and lines[0].startswith("# Changelog"):
        lines = lines[1:]
    body = "\n".join(lines).rstrip("\n")

    archives = sorted(glob.glob(os.path.join(dist, "*.tar.gz")) +
                      glob.glob(os.path.join(dist, "*.zip")))
    if not archives:
        die(f"{dist}/ 下没有 .tar.gz / .zip 产物 (先跑 mage pack)")

    checks = [f"- {sha1(p)}  {os.path.basename(p)}" for p in archives]
    body += "\n\n## checksum files\n\n" + "\n".join(checks) + "\n"
    return body, archives


def main():
    if len(sys.argv) < 2:
        die("用法: publish_release.py <tag> [repo] [dist_dir]")
    tag = sys.argv[1]
    repo = sys.argv[2] if len(sys.argv) > 2 else "cnphpbb/mysshw"
    dist = sys.argv[3] if len(sys.argv) > 3 else "dist"

    token = get_token()

    # 1. release 是否已存在 (幂等)
    code, body = api("GET", f"{API}/repos/{repo}/releases/tags/{tag}", token)
    if code == 200:
        rel = json.loads(body)
        print(f"release {tag} 已存在 (id={rel['id']}) — 跳过创建")
        print(f"  {rel['html_url']}")
        print("如需补传产物, 删掉本脚本的跳过逻辑或直接用 gh release upload")
        return 0
    if code != 404:
        die(f"查询 release 失败 HTTP {code}: {body[:400].decode('utf-8', 'ignore')}")

    # 2. 组装 body
    notes, archives = build_body(tag, dist)
    print(f"body: {len(notes)} bytes, {len(archives)} archives")

    # 3. 创建 release
    code, body = api("POST", f"{API}/repos/{repo}/releases", token, payload={
        "tag_name": tag,
        "target_commitish": "main",
        "name": tag,
        "body": notes,
        "draft": False,
        "prerelease": False,
    })
    if code != 201:
        die(f"创建 release 失败 HTTP {code}: {body[:600].decode('utf-8', 'ignore')}")
    rel = json.loads(body)
    print(f"created: {rel['html_url']}")

    # 4. 上传产物
    upload_url = rel["upload_url"].split("{")[0]
    files = archives + [os.path.join(dist, "checksum.txt")]
    for path in files:
        if not os.path.isfile(path):
            print(f"  skip (不存在): {path}")
            continue
        name = os.path.basename(path)
        with open(path, "rb") as f:
            data = f.read()
        code, resp = api(
            "POST",
            f"{upload_url}?name={name}",
            token,
            raw=data,
            content_type="application/octet-stream",
        )
        if code != 201:
            print(f"  FAIL {name}: HTTP {code} {resp[:200].decode('utf-8', 'ignore')}")
            continue
        print(f"  ok  {name}  ({len(data):,} bytes)")

    print(f"\n发布完成: {rel['html_url']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
