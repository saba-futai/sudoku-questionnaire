# Sudoku Questionnaire

A questionnaire platform where **every answer is stored as a sudoku puzzle**.

The browser encodes each response with a sudoku cipher before it is sent; the
database holds nothing but ciphertext. When an administrator browses, filters,
exports or edits, the server decodes and shows the real content. Uploaded images
are stored as base64 — and that base64 is itself sudoku-encoded first.

```
plaintext ──one uniquely-solvable 4-clue puzzle per byte──▶ ciphertext ──▶ SQLite (TEXT)
                                                                              │
administrator / export ◀────── decode ────── plaintext ◀──────────────────────┘
```

The encoding is a port of the appearance layer from
[`SUDOKU-ASCII/sudoku`](https://github.com/SUDOKU-ASCII/sudoku) (`pkg/obfs/sudoku`).
The normative specification lives in [`spec/CODEC.md`](spec/CODEC.md).

---

## Contents

- [How the encoding works](#how-the-encoding-works)
- [Features](#features)
- [One-click deploy](#one-click-deploy)
- [Manual setup](#manual-setup)
- [Configuration](#configuration)
- [Architecture](#architecture)
- [Verifying the codec](#verifying-the-codec)
- [Security properties — read this](#security-properties--read-this)
- [Credits and licence](#credits-and-licence)

---

## How the encoding works

A 4×4 sudoku has **288** valid grids — more than the 256 possible byte values.
That surplus is the whole trick.

1. **Enumerate** all 288 grids, and all **1820** ways to choose 4 clue cells out
   of 16. Both enumerations are fixed constants.
2. **Map** each byte value to one grid, chosen by shuffling the 288 with a PRNG
   seeded from the questionnaire's key.
3. **Encode** a byte by taking that grid's four clue cells — a set chosen so the
   four clues *uniquely determine the grid* among all 288. Emit those four clues
   as wire symbols, in one of 24 randomly chosen orders, with optional filler.
4. **Decode** by collecting the clue symbols, sorting the four in each group,
   and looking the group up to recover the byte.

Because each byte has **12–128 equally valid encodings**, and the encoder picks
one at random each time, the same answer produces a *different* ciphertext on
every submission. Nothing in the database can be clustered or compared by
ciphertext alone.

A four-symbol trailer carries a 24-bit checksum of the plaintext, and the
decoder accepts only an exact symbol alphabet. Together these turn a wrong key,
a wrong layout, truncation or tampering into a **loud error** instead of
plausible-looking garbage.

Payload expansion is a predictable **4×** (plus a 4-symbol trailer).

> **Not wire-compatible with the upstream tunnel.** The principle is preserved
> line for line, but the table shuffle uses a small language-neutral PRNG
> (splitmix64) instead of Go's `math/rand`, whose lagged-Fibonacci source cannot
> be reproduced reliably in JavaScript — and a silent mismatch there would mean
> the browser encodes data the server cannot read. See
> [`spec/CODEC.md`](spec/CODEC.md) §0 for the full list of deliberate
> differences.

## Features

**For respondents** — a multi-step form with a progress indicator and slide
transitions; single / multi choice, dropdown, short text, long text, rating,
date and image questions; per-step validation; client-side image compression
before encoding; a QR code per questionnaire for sharing the fill link.

**For administrators** — a question builder with a live preview; a dashboard
with statistics; a decoded response table with search, paging, inline editing
and deletion; charts per question type; export to **CSV / XLSX / JSON** with the
real answers (images embedded in the spreadsheet); an audit log; and a
**codec inspector** that shows a live plaintext ↔ ciphertext translator, the
four clue cells chosen for each byte, and the literal ciphertext as stored next
to its decoded form.

**Operationally** — one static binary with the frontend embedded, so the target
host needs no Node, no nginx and no package downloads. Runs unprivileged with
only `CAP_NET_BIND_SERVICE`.

## One-click deploy

Designed to run on a fresh Debian/Ubuntu host. It deliberately binds
**3388** (and **3389**) by default so it never collides with a web server
already holding port 80.

```bash
git clone https://github.com/saba-futai/sudoku-questionnaire.git
cd sudoku-questionnaire
sudo ./deploy/install.sh
```

`install.sh` will:

1. build the frontend and cross-compile a static binary (needs **Go ≥ 1.24** and
   **Node ≥ 20**; pass `--binary /path/to/qrforqa-linux-amd64` to skip this and
   use a prebuilt binary instead);
2. create an unprivileged `qrforqa` system user;
3. generate a master key, a JWT secret and an admin password into
   `/etc/qrforqa/qrforqa.env` (mode `0600`);
4. install and start a hardened systemd unit;
5. print the generated admin credentials **once**.

Then open `http://<host>:3388`.

If your host has a firewall, allow the ports:

```bash
sudo ufw allow 3388/tcp && sudo ufw allow 3389/tcp
# or, with nftables/iptables directly, plus your cloud provider's security group
```

> **Back up `/etc/qrforqa/qrforqa.env` immediately.** It contains
> `QRFORQA_MASTER_KEY`, which derives every questionnaire's mapping table.
> Lose it and every stored answer becomes permanently unreadable.

### Deploying from a development machine instead

If you would rather build locally and push the artifact:

```bash
SSHPASS='…' DEPLOY_HOST=my.host DEPLOY_PORT=22 ./scripts/deploy.sh
```

This cross-compiles a static `linux/amd64` binary, streams it over SSH, installs
the unit and re-verifies the deployment end to end.

## Manual setup

**Requirements:** Go ≥ 1.24, Node ≥ 20 (build only). Nothing at runtime.

```bash
# 1. secrets
cd backend
go run ./cmd/server keygen          # prints a master key

# 2. run the API
QRFORQA_ADDR=:8080 \
QRFORQA_DATA_DIR=./data \
QRFORQA_MASTER_KEY=<the key above> \
QRFORQA_ADMIN_PASSWORD=<at least 8 characters> \
go run ./cmd/server serve

# 3. in another terminal, the dev frontend (proxies /api to :8080)
cd frontend && npm install && npm run dev      # http://localhost:5173
```

Production build:

```bash
make build          # local binary
make build-linux    # static linux/amd64 binary
```

## Configuration

All configuration is environment variables; see
[`deploy/qrforqa.env.example`](deploy/qrforqa.env.example).

| Variable | Default | Notes |
|---|---|---|
| `QRFORQA_MASTER_KEY` | — | **Required.** Seeds every questionnaire's table. Keep it secret and backed up. |
| `QRFORQA_ADMIN_PASSWORD` | — | Required on first run only; seeds the admin account. Change it later in Settings. |
| `QRFORQA_ADDR` | `:8080` | Primary listener. The shipped unit sets `:3388`. |
| `QRFORQA_ALT_ADDR` | — | Optional second listener (the unit sets `:3389`). Empty disables it. |
| `QRFORQA_DATA_DIR` | `./data` | Data directory. |
| `QRFORQA_DB` | `<data>/qrforqa.db` | SQLite path. |
| `QRFORQA_ADMIN_USER` | `admin` | Admin username. |
| `QRFORQA_JWT_SECRET` | derived | Session signing key. |
| `QRFORQA_TOKEN_TTL` | `12h` | Session lifetime. |
| `QRFORQA_MAX_UPLOAD_BYTES` | `2097152` | Cap on one image's *ciphertext* (≈4× base64, ≈5.3× the raw image). |
| `QRFORQA_LAYOUT` | `ascii` | `ascii` only. `entropy` is rejected at startup — see below. |

`QRFORQA_LAYOUT=entropy` emits control bytes and bytes ≥ 0x80. Ciphertext is
stored and transported as a JSON string, and JSON rewrites invalid UTF-8 to
U+FFFD, which would silently corrupt it. The entropy layout exists in the codec
library and in the inspector; the application refuses to start with it rather
than corrupt data later.

## Architecture

```
spec/               CODEC.md (normative) + cross-language test vectors
backend/            Go service
  internal/codec/     the sudoku codec (grids, prng, validity, layouts, tables)
  internal/vault/     the plaintext↔ciphertext boundary; caches one table per questionnaire
  internal/schema/    definition format + answer validation (runs on decoded values)
  internal/services/  questionnaires, responses, statistics, export
  internal/webui/     the embedded frontend
frontend/           Vue 3 SPA
  src/codec/          the TypeScript mirror of the codec, bit-identical to Go
  src/stores/codec.ts the browser encode pipeline (Web Worker + fingerprint check)
scripts/            conformance.sh · smoke.py · deploy.sh
deploy/             install.sh · systemd unit · env template
```

**Stack** — frontend: Vue 3.5, Vite 8, TypeScript, Pinia, Tailwind CSS 4,
motion-v, ECharts 6. Backend: Go, Gin, GORM, SQLite (`glebarez/sqlite`, pure Go,
no CGO) and excelize. Deployment: one static binary under systemd.

**Per-questionnaire keys.** The master key never leaves the server. Each
questionnaire's table is derived as `sha256(masterKey + "|q:" + id)`, and the
browser receives only that derived key — so one compromised questionnaire cannot
read any other's data. A questionnaire is committed to a stable ID before its
first encode, so editing questions never invalidates existing answers.

## Verifying the codec

A silent divergence between the Go and TypeScript implementations would mean the
browser encodes data the server cannot decode. Four independent gates prevent it:

```bash
make conformance   # regenerate artifacts, fail if the checked-in vectors are stale,
                   # then run the Go and TypeScript suites
make check         # vet + typecheck + all tests
python3 scripts/smoke.py http://127.0.0.1:8080   # 68 end-to-end API checks
cd frontend && npm run e2e                        # 14 browser tests, desktop + mobile
```

1. `spec/CODEC.md` is the single normative source.
2. `spec/vectors/*.json` pin the grid hash, the PRNG stream, table fingerprints
   and full plaintext↔ciphertext pairs.
3. **Bidirectional cross-execution** — the Go CLI and the TypeScript
   implementation encode the same inputs and the test asserts the output bytes
   are *identical*. That is strictly stronger than a round-trip: it also catches
   a divergence where each side is self-consistent but different.
4. **Runtime self-test** — the server asserts its derivation matches pinned
   constants at startup and **refuses to boot** otherwise; the browser verifies
   the bitmap digest, then checks its table fingerprint against the server's and
   shows a banner if they disagree.

The Python smoke test additionally asserts that the production database contains
no plaintext: it searches every business table for the markers it just
submitted.

## Security properties — read this

**This encoding is obfuscation, not encryption.**

The key only permutes an array of 288 grids, and those grids are public. Anyone
who knows the scheme can see which position sets appear and, because a grid has
either 12 or 128 valid encodings, infer which symmetry orbit each byte belongs
to. The browser bundle carries the derived key, so anyone holding the frontend
can decode.

What it *does* give you:

- the database genuinely contains only sudoku ciphertext, with no plaintext
  business data (only a bcrypt hash and a username);
- per-questionnaire keys make the same answer look different in every
  questionnaire, so data cannot be compared across them;
- randomised encoding makes identical answers differ within a questionnaire, so
  responses cannot be clustered by ciphertext.

What it does *not* give you: cryptographic confidentiality. If you need that,
wrap the payload in an AEAD **underneath** the sudoku layer — which is exactly
what the upstream tunnel does (AEAD for secrecy, sudoku for appearance).
This project deliberately does not, per its requirements.

Operational hardening that *is* in place: bcrypt password hashing, a per-username
login lockout, rate limiting on submission and upload, JWT sessions, a
non-root systemd unit with a single capability, `ProtectSystem=strict`, and a
memory cap.

## Credits and licence

The sudoku codec is derived from
[`SUDOKU-ASCII/sudoku`](https://github.com/SUDOKU-ASCII/sudoku)
(`pkg/obfs/sudoku`), Copyright (C) 2026 by saba, GPL-3.0. Reference commit:
`4889b53cb35355123bebd40e6c76a9582de7c23d`. The differences from that
implementation are enumerated in [`spec/CODEC.md`](spec/CODEC.md) §0.

This project is licensed under the **MIT License** — see [`LICENSE`](LICENSE).

The upstream codec is GPL-3.0 and is authored by the same copyright holder as
this project, who is therefore free to license their own derivative work
permissively. A third party redistributing the upstream codec itself remains
bound by its GPL-3.0 terms.
