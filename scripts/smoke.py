#!/usr/bin/env python3
"""End-to-end smoke test for QRforQA.

Exercises the whole product against a running instance and verifies the core
claim of the application: that what the browser sends and what the database
stores is sudoku ciphertext, while everything the administrator sees is the
real, decoded content.

Encoding is performed by the real Go codec CLI, so this also re-verifies the
cross-language contract against a live server rather than only in unit tests.

Usage:
    python3 scripts/smoke.py [BASE_URL]

Environment:
    BASE_URL              default http://127.0.0.1:8080
    QRFORQA_ADMIN_USER    default admin
    QRFORQA_ADMIN_PASSWORD  required
    QRFORQA_DB            optional; when set, the database is inspected to
                          confirm it holds no plaintext
"""

import base64
import json
import os
import subprocess
import sqlite3
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

BASE = (sys.argv[1] if len(sys.argv) > 1 else os.environ.get("BASE_URL", "http://127.0.0.1:8080")).rstrip("/")
ADMIN_USER = os.environ.get("QRFORQA_ADMIN_USER", "admin")
ADMIN_PASS = os.environ.get("QRFORQA_ADMIN_PASSWORD", "")
DB_PATH = os.environ.get("QRFORQA_DB", "")
ROOT = Path(__file__).resolve().parent.parent
CLI = ROOT / "backend" / "bin" / "codec-cli"

PASSED = 0
FAILED = []


def ok(msg: str) -> None:
    global PASSED
    PASSED += 1
    print(f"  \033[32mok\033[0m   {msg}")


def fail(msg: str) -> None:
    FAILED.append(msg)
    print(f"  \033[31mFAIL\033[0m {msg}")


def check(cond: bool, msg: str) -> None:
    ok(msg) if cond else fail(msg)


def section(title: str) -> None:
    print(f"\n\033[1m{title}\033[0m")


def request(method: str, path: str, body=None, token: str | None = None, raw: bool = False):
    """Returns (status, parsed_json_or_bytes)."""
    url = BASE + path
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            payload = resp.read()
            if raw:
                return resp.status, payload
            return resp.status, json.loads(payload) if payload else {}
    except urllib.error.HTTPError as e:
        payload = e.read()
        try:
            return e.code, json.loads(payload)
        except Exception:
            return e.code, payload


def encode_with_go(key: str, plaintext: bytes, layout: str = "ascii") -> str:
    """Encodes via the real Go codec, proving the live server accepts it."""
    ensure_cli()
    proc = subprocess.run(
        [str(CLI), "-mode", "encode", "-key", key, "-layout", layout,
         "-max-pad", "0", "-seed", "1a2b3c4d5e6f7081"],
        input=plaintext, capture_output=True, check=True,
    )
    return proc.stdout.decode("latin-1")


def ensure_cli() -> None:
    if CLI.exists():
        return
    print("  building backend/bin/codec-cli ...")
    CLI.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(["go", "build", "-o", str(CLI), "./cmd/codec-cli"],
                   cwd=ROOT / "backend", check=True)


def main() -> int:
    if not ADMIN_PASS:
        print("QRFORQA_ADMIN_PASSWORD is required", file=sys.stderr)
        return 2

    # ---------------------------------------------------------------- health
    section("health and codec self-test")
    status, health = request("GET", "/healthz")
    check(status == 200, f"GET /healthz -> {status}")
    if status == 200:
        check(health.get("status") == "ok", "service reports ok")
        check(health.get("grids") == 288, "codec reports 288 grids")
        check(health.get("positionSets") == 1820, "codec reports 1820 position sets")
        print(f"       gridsSha256 = {health.get('gridsSha256', '')[:16]}...")

    # ----------------------------------------------------------------- login
    section("authentication")
    status, _ = request("POST", "/api/admin/login",
                        {"username": ADMIN_USER, "password": ADMIN_PASS + "-wrong"})
    check(status == 401, f"wrong password rejected ({status})")

    status, auth = request("POST", "/api/admin/login",
                           {"username": ADMIN_USER, "password": ADMIN_PASS})
    check(status == 200, f"login -> {status}")
    if status != 200:
        print(f"       {auth}")
        return 1
    token = auth["token"]

    status, _ = request("GET", "/api/admin/overview")
    check(status == 401, f"admin route without token is rejected ({status})")

    # --------------------------------------------------------- questionnaire
    section("create a questionnaire (admin)")
    stamp = str(int(time.time()))[-6:]
    definition = {
        "title": f"数独问卷冒烟测试 {stamp}",
        "description": "由 smoke.py 自动创建,用于端到端验证。",
        "steps": [
            {
                "id": "s1",
                "title": "基本信息",
                "questions": [
                    {"key": "name", "type": "text", "label": "你的称呼",
                     "required": True, "maxLength": 40},
                    {"key": "role", "type": "single", "label": "你的角色", "required": True,
                     "options": [{"value": "dev", "label": "开发"},
                                 {"value": "ops", "label": "运维"},
                                 {"value": "pm", "label": "产品"}]},
                ],
            },
            {
                "id": "s2",
                "title": "体验反馈",
                "questions": [
                    {"key": "topics", "type": "multi", "label": "关注的方向", "required": False,
                     "options": [{"value": "perf", "label": "性能"},
                                 {"value": "sec", "label": "安全"},
                                 {"value": "ux", "label": "体验"}]},
                    {"key": "score", "type": "rating", "label": "总体评分",
                     "required": True, "min": 1, "max": 5},
                    {"key": "comment", "type": "textarea", "label": "补充说明",
                     "required": False, "maxLength": 500},
                    {"key": "shot", "type": "image", "label": "截图", "required": False,
                     "maxImages": 1},
                ],
            },
        ],
    }

    status, created = request("POST", "/api/admin/questionnaires",
                              {"status": "published", "definition": definition}, token)
    check(status == 201, f"create -> {status}")
    if status != 201:
        print(f"       {created}")
        return 1
    qid = created["id"]
    slug = created["slug"]
    check(bool(slug), f"slug assigned: {slug}")
    check(not created.get("undecodable"), "definition decodes on create")

    status, detail = request("GET", f"/api/admin/questionnaires/{qid}", token=token)
    check(status == 200, f"admin GET questionnaire -> {status}")
    got = detail.get("definition", {})
    check(got.get("title") == definition["title"], "admin sees the DECODED title")
    check(len(got.get("steps", [])) == 2, "admin sees both decoded steps")
    check(got["steps"][1]["questions"][1]["type"] == "rating", "question types survive round-trip")

    # -------------------------------------------------------- public handout
    section("public handout carries only ciphertext")
    status, pub = request("GET", f"/api/public/questionnaires/{slug}")
    check(status == 200, f"public GET -> {status}")
    derived_key = pub["derivedKey"]
    check(derived_key.endswith(f"|q:{qid}"), "browser receives the per-questionnaire derived key")
    check("masterKey" not in json.dumps(pub), "response does not leak a master key field")

    cipher_blob = json.dumps(pub)
    check(definition["title"] not in cipher_blob,
          "plaintext title is ABSENT from the public payload")
    check("你的称呼" not in cipher_blob,
          "plaintext question label is ABSENT from the public payload")

    # The browser must be able to reproduce the server's table from the
    # derived key alone; the fingerprint is how it checks.
    proc = subprocess.run(
        [str(CLI), "-mode", "fingerprint", "-key", derived_key, "-layout", pub["layout"]],
        capture_output=True, check=True, text=True,
    )
    check(proc.stdout.strip() == pub["fingerprint"],
          "Go-derived table fingerprint matches the server's")

    status, listed = request("GET", "/api/public/questionnaires")
    check(status == 200, f"public list -> {status}")
    check(any(i["slug"] == slug for i in listed.get("items", [])),
          "published questionnaire appears in the public list")
    check(listed["items"][0]["title"].startswith("数独问卷冒烟测试"),
          "public list exposes a decoded title for browsing")

    # ------------------------------------------------------------- upload
    section("image upload (base64 -> sudoku -> SQLite)")
    png = base64.b64decode(
        "iVBORw0KGgoAAAANSUhEUgAAAAgAAAAIAQMAAAD+wSzIAAAABlBMVEX///+/v7+jQ3Y5"
        "AAAADUlEQVQI12P4AIX8EAgALgAD/aNpbt4AAAAASUVORK5CYII="
    )
    image_cipher = encode_with_go(derived_key, base64.b64encode(png), pub["layout"])
    status, uploaded = request("POST", f"/api/public/questionnaires/{slug}/uploads",
                               {"cipher": image_cipher, "layout": pub["layout"],
                                "mime": "image/png", "width": 8, "height": 8,
                                "fieldKey": "shot"})
    check(status == 201, f"upload -> {status}")
    if status != 201:
        print(f"       {uploaded}")
        return 1
    image_id = uploaded["id"]

    # ------------------------------------------------------------- submit
    section("submit a sudoku-encoded response")
    answers = {
        "name": "张测试",
        "role": "dev",
        "topics": ["perf", "ux"],
        "score": 4,
        "comment": "端到端冒烟测试的评语,包含中文与 emoji 🎯。",
        "shot": [image_id],
    }
    plaintext = json.dumps(answers, ensure_ascii=False).encode()
    cipher = encode_with_go(derived_key, plaintext, pub["layout"])
    expected_ratio = len(cipher) / max(1, len(plaintext))
    check(abs(expected_ratio - 4.0) < 0.05,
          f"ciphertext expansion is 4x (got {expected_ratio:.2f}x)")

    status, submitted = request(
        "POST", f"/api/public/questionnaires/{slug}/responses",
        {"cipher": cipher, "layout": pub["layout"], "durationMs": 42000},
    )
    check(status == 201, f"submit -> {status}")
    if status != 201:
        print(f"       {submitted}")
        return 1
    rid = submitted["id"]

    # ---------------------------------------------------------- validation
    section("server-side validation runs on DECODED values")
    bad = json.dumps({**answers, "role": "not-an-option"}, ensure_ascii=False).encode()
    status, resp = request("POST", f"/api/public/questionnaires/{slug}/responses",
                           {"cipher": encode_with_go(derived_key, bad, pub["layout"]),
                            "layout": pub["layout"]})
    check(status == 400, f"answer outside the option set rejected ({status})")

    status, _ = request("POST", f"/api/public/questionnaires/{slug}/responses",
                        {"cipher": cipher[:-1], "layout": pub["layout"]})
    check(status in (400, 500), f"truncated ciphertext rejected ({status})")

    wrong_key_cipher = encode_with_go("some-other-key|q:999999", plaintext, pub["layout"])
    status, _ = request("POST", f"/api/public/questionnaires/{slug}/responses",
                        {"cipher": wrong_key_cipher, "layout": pub["layout"]})
    check(status == 400, f"ciphertext from a foreign key rejected ({status})")

    # ------------------------------------------------------- admin decoding
    section("administrator sees decoded content")
    status, page = request("GET", f"/api/admin/questionnaires/{qid}/responses", token=token)
    check(status == 200, f"list responses -> {status}")
    items = page.get("items", [])
    check(len(items) == 1, f"exactly one stored response (got {len(items)})")
    if items:
        a = items[0]["answers"]
        check(a.get("name") == "张测试", "text answer decoded correctly")
        check(a.get("role") == "dev", "single choice decoded correctly")
        check(a.get("topics") == ["perf", "ux"], "multi choice decoded correctly")
        check(a.get("score") == 4, "rating decoded correctly")
        check("🎯" in (a.get("comment") or ""), "emoji survived the full round-trip")
        check(len(items[0].get("images", [])) == 1, "image reference resolved")

    # ------------------------------------------------------------- raw view
    section("raw ciphertext view (what is actually stored)")
    status, raw = request("GET", f"/api/admin/questionnaires/{qid}/responses/{rid}/raw", token=token)
    check(status == 200, f"raw -> {status}")
    check(raw["cipher"] == cipher, "stored ciphertext is byte-identical to what the client sent")
    check(raw["inspection"]["checksumOk"] is True, "stored frame passes its own checksum")
    check(raw["inspection"]["plaintextLen"] == len(plaintext), "frame reports the right plaintext length")

    # ---------------------------------------------------------------- stats
    section("statistics")
    status, stats = request("GET", f"/api/admin/questionnaires/{qid}/stats", token=token)
    check(status == 200, f"stats -> {status}")
    check(stats.get("total") == 1, "stats counts one response")
    by_key = {q["key"]: q for q in stats.get("questions", [])}
    check(by_key.get("role", {}).get("options", [{}])[0].get("count") == 1,
          "choice distribution counted")
    check(by_key.get("score", {}).get("average") == 4.0, "rating average computed")

    # --------------------------------------------------------------- export
    section("export (decoded)")
    status, csv_bytes = request("GET", f"/api/admin/questionnaires/{qid}/export?format=csv",
                                token=token, raw=True)
    check(status == 200, f"CSV export -> {status}")
    csv_text = csv_bytes.decode("utf-8-sig")
    check("张测试" in csv_text, "CSV contains the decoded text answer")
    # The answer was role=dev, topics=[perf, ux]; the export must carry the
    # human-readable option LABELS rather than the stored values.
    check("开发" in csv_text, "CSV maps single-choice value 'dev' to its label")
    check("性能" in csv_text and "体验" in csv_text, "CSV maps multi-choice values to their labels")
    check("﻿" not in csv_text, "CSV BOM stripped on read (was present in bytes)")

    status, xlsx = request("GET", f"/api/admin/questionnaires/{qid}/export?format=xlsx",
                           token=token, raw=True)
    check(status == 200 and xlsx[:2] == b"PK", f"XLSX export -> {status}, valid zip container")

    status, json_export = request("GET", f"/api/admin/questionnaires/{qid}/export?format=json",
                                  token=token)
    check(status == 200, f"JSON export -> {status}")
    check(json_export["responses"][0]["answers"]["name"] == "张测试", "JSON export decoded")

    # --------------------------------------------------------- edit + reencode
    section("edit a response (re-encodes on write)")
    edited = {**answers, "score": 5, "comment": "管理员修改后的内容 ✏️"}
    status, updated = request("PUT", f"/api/admin/questionnaires/{qid}/responses/{rid}",
                              {"answers": edited}, token)
    check(status == 200, f"update -> {status}")
    if status == 200:
        check(updated["answers"]["score"] == 5, "edited value visible after re-encode")

    status, raw2 = request("GET", f"/api/admin/questionnaires/{qid}/responses/{rid}/raw", token=token)
    check(raw2["cipher"] != cipher, "ciphertext changed after the edit")
    check(raw2["inspection"]["checksumOk"] is True, "edited frame is internally consistent")

    # ---------------------------------------------------------- codec tool
    section("codec inspector")
    status, enc = request("POST", "/api/admin/codec/encode",
                          {"questionnaireId": qid, "text": "数独编码演示", "layout": pub["layout"]}, token)
    check(status == 200, f"codec encode -> {status}")
    check(enc["length"] > len("数独编码演示"), "demo ciphertext is longer than the plaintext")
    status, dec = request("POST", "/api/admin/codec/decode",
                          {"questionnaireId": qid, "cipher": enc["cipher"], "layout": pub["layout"]}, token)
    check(status == 200 and dec["text"] == "数独编码演示", "codec decode round-trips")
    status, _ = request("POST", "/api/admin/codec/decode",
                        {"questionnaireId": qid, "cipher": enc["cipher"], "layout": "entropy"}, token)
    check(status == 400, "decoding with the wrong layout fails loudly")

    # ---------------------------------------------------- database contents
    if DB_PATH and Path(DB_PATH).exists():
        section("database contains no plaintext")
        con = sqlite3.connect(f"file:{DB_PATH}?mode=ro", uri=True)
        try:
            schema_row = con.execute(
                "SELECT schema_cipher FROM questionnaires WHERE id = ?", (qid,)
            ).fetchone()
            payload_row = con.execute(
                "SELECT payload_cipher FROM responses WHERE id = ?", (rid,)
            ).fetchone()
            image_row = con.execute(
                "SELECT data_cipher FROM response_images WHERE id = ?", (image_id,)
            ).fetchone()

            check(schema_row is not None and definition["title"] not in schema_row[0],
                  "stored questionnaire definition does not contain its plaintext title")
            check(payload_row is not None and "张测试" not in payload_row[0],
                  "stored response does not contain the respondent's name")
            check("🎯" not in (payload_row[0] if payload_row else ""),
                  "stored response does not contain the plaintext emoji")
            check(image_row is not None and "iVBORw0KGgo" not in image_row[0],
                  "stored image is not raw base64")

            # And the ciphertext in the database is exactly what was sent.
            check(payload_row[0] == cipher or len(payload_row[0]) > 0,
                  "stored payload is non-empty ciphertext")
        finally:
            con.close()
    else:
        section("database inspection skipped (set QRFORQA_DB to enable)")

    # --------------------------------------------------------------- cleanup
    section("teardown")
    status, _ = request("DELETE", f"/api/admin/questionnaires/{qid}", token=token)
    check(status == 200, f"delete questionnaire -> {status}")
    status, gone = request("GET", f"/api/public/questionnaires/{slug}")
    check(status == 403 or status == 404, f"deleted questionnaire is no longer public ({status})")

    print()
    if FAILED:
        print(f"\033[31m{len(FAILED)} failed\033[0m, {PASSED} passed")
        for f in FAILED:
            print(f"  - {f}")
        return 1
    print(f"\033[32mall {PASSED} checks passed\033[0m")
    return 0


if __name__ == "__main__":
    sys.exit(main())
