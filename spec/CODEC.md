# QRforQA 数独编解码规范 v1

本文件是**规范性**的。`backend/internal/codec`(Go)与 `frontend/src/codec`(TypeScript)
是它的两份实现,任何一方偏离本文件都会导致前端编码、后端解不开的静默数据损坏。
两份实现必须对 `spec/vectors/` 下的同一批向量产生完全一致的结果。

关键词 MUST / MUST NOT / SHOULD 按 RFC 2119 解释。

---

## 0. 与上游的关系

原理取自 [`SUDOKU-ASCII/sudoku`](https://github.com/SUDOKU-ASCII/sudoku) 的 `pkg/obfs/sudoku`:

| 保留(逐条对齐) | 说明 |
|---|---|
| 288 个 4×4 数独网格的回溯枚举 | §2 |
| 1820 个位置组合(C(16,4),字典序) | §3 |
| 「四点唯一解」判定 | §4 |
| 排序打包 → 查表 的解码方式 | §7 |
| 24 种乱序 + 填充池 的混淆 | §8 |
| ASCII / 熵 两种字节布局 | §6 |

| 有意偏离 | 理由 |
|---|---|
| **不使用 Go 的 `math/rand`** 洗牌,改用 splitmix64(§5) | `math/rand` 的 lagged-Fibonacci 源(607 项 `rngCooked`)+ `Int31n` 取模循环无法在 JS 里可靠复刻;复刻失败的后果是静默数据损坏。 |
| 解码表按 **6-bit group** 打包,而非原始提示字节(§7) | 避免 JS 中 `a << 24` 对 `a ≥ 128` 变负数,并使解码表与布局无关。 |
| 提示字节判定用**精确字母表成员**而非位掩码(§6.3) | 上游的 `(b & 0x40) == 0x40` 会把 `0x80..0xFF` 也当提示字节,任何混入的 UTF-8 文本会被静默吞掉。 |
| 增加 **CRC-24 尾帧**(§9) | 上游没有完整性校验。没有它,错密钥、错布局、传输损坏全部表现为「看起来合理的乱码」。 |

**后果:本应用与 `SUDOKU-ASCII/sudoku` 隧道不共享映射表。**
上游参照 commit:`4889b53cb35355123bebd40e6c76a9582de7c23d`(2026-09-09)。

---

## 1. 术语

- **group(组)**:一个线索的 6 位编码,`(digit-1) << 4 | pos`,取值 0..63。
- **hint byte(提示字节)**:group 在具体布局下的线缆字节。
- **body**:N 个明文字节编码成的 4N 个提示字节。
- **frame(帧)**:body 加上 4 个尾帧提示字节。

---

## 2. 网格枚举

MUST 用回溯法枚举全部合法 4×4 数独(2×2 宫)。遍历顺序是**载荷**,不是细节。

```
backtrack(idx):
  if idx == 16: emit g; return
  row = idx / 4 ; col = idx % 4
  br = (row/2)*2 ; bc = (col/2)*2
  for num in 1..4:
    valid = 行 row 无 num 且 列 col 无 num          # 行/列先于宫检查
    if valid: 宫 (br,bc) 内无 num
    if valid: g[idx] = num; backtrack(idx+1); g[idx] = 0
```

结果 MUST 为 **288** 个网格。`sha256(拼接 288 个网格,每个 16 字节)` 钉在
`spec/vectors/structure.json:gridsSha256`。

> 顺序为何是载荷:洗牌置换的是**下标**,不同的遍历顺序会得到 288 个「正确但次序不同」的网格,
> 从而整张表不同。

---

## 3. 位置组合枚举

MUST 按字典序枚举全部 C(16,4)=**1820** 个上升四元组:

```
for a in 0..12: for b in a+1..13: for c in b+1..14: for d in c+1..15: emit {a,b,c,d}
```

下标 0 是 `{0,1,2,3}`,下标 1819 是 `{12,13,14,15}`(末位变化最快)。
哈希钉在 `structure.json:posSetsSha256`。

---

## 4. 有效性(与密钥无关)

对每个网格 g 与每个位置组合 p,取其四格的 group,排序打包成 24 位签名(§7.1)。

`p` 对 `g` 有效 ⟺ 在**全部 288 个网格**中,恰有 1 个网格在 `p` 的四格上与 `g` 一致。

**本性质与密钥无关**:它只依赖 §2 与 §3 两个常量。因此这 524,160 次签名计算是**构建期产物**,
而非每份问卷的运行时开销。字节 b 的合法编码直接继承自它所映射到的网格。

产物为 `backend/internal/codec/tables/validity.bin`:288 行 × 228 字节 = **65,664 字节**位图,
第 g 行第 p 位表示 (g,p) 是否有效。同时作为静态资源下发给前端。

实测结构:`|P_g| ∈ {12, 128}`,96 个网格 12、192 个网格 128 —— 正是 4×4 数独的两个对称轨道,
可作为枚举正确性的独立旁证。

Go 侧 MUST 提供 `ComputeValidity()` 从零重算,并有测试断言它与嵌入的 `validity.bin` 逐字节相同。
TS 侧同样 MUST 提供 `computeValidity()`,并断言与下载的位图相同 —— 这样前端仍是完整实现,
且运行时不付重算代价。

---

## 5. 密钥派生与 PRNG

### 5.1 派生

```
key  = masterKey + "|q:" + questionnaireID
seed = be64( sha256( "QRforQA:sudoku-codec:v1" || 0x00 || utf8(key) )[0:8] )
```

- 域分隔串 MUST 存在(避免同一密钥材料被跨用途复用)。
- MUST 对 **UTF-8 字节**求哈希,而不是 JS 字符串(非 ASCII 密钥的经典静默分歧点)。
- 空密钥 MUST 仍然哈希「域分隔串 + 0x00」,而不是返回 0。

### 5.2 splitmix64

全部运算按 mod 2⁶⁴。移位 MUST 为逻辑移位(补零)。

```
state = seed
nextUint64():
    state = (state + 0x9E3779B97F4A7C15) mod 2^64
    z = state
    z = ((z xor (z >> 30)) * 0xBF58476D1CE4E5B9) mod 2^64
    z = ((z xor (z >> 27)) * 0x94D049BB133111EB) mod 2^64
    return z xor (z >> 31)

nextUint32() = nextUint64() >> 32          # 取【高】32 位,钉死

nextBelow(n):                               # 要求 1 <= n <= 2^32-1
    if n <= 1: return 0
    limit = (2^32 / n) * n                  # 整数除法,再乘回来
    loop:
        x = nextUint32()
        if x < limit: return x mod n

shuffle(a):                                 # 就地,降序 Fisher–Yates
    for i = len(a)-1 downto 1:
        swap(a[i], a[nextBelow(i+1)])

pick(a) = a[nextBelow(len(a))]
```

### 5.3 已知陷阱(实现 MUST 规避)

| # | 陷阱 | 规则 |
|---|---|---|
| 1 | Go 中 `limit` 用 `uint32` 计算 | `(1<<32)/n*n` 在 uint32 下**静默得 0**,`x < limit` 恒假 → **死循环**。必须用 `uint64(1)<<32`。 |
| 2 | JS 的 `>>` 是算术右移 | 全程用 `>>>`,每个运算后 `>>> 0` 归一化。 |
| 3 | `Math.imul` 返回**有符号** int32 | 结果必须 `>>> 0` 再使用。 |
| 4 | JS 用 Number 存 u64 状态 | 存不下。MUST 用 `(lo, hi)` 双 32 位或 BigInt。 |
| 5 | JS 中用 `a * b` 做 32 位乘法 | 精度丢失。只能用 `Math.imul`,或 16 位拆分的 `umulh`。 |
| 6 | 64 位右移拼接方向写反 | 低位 `(z0 >>> k) \| (z1 << (32-k))`,高位 `z1 >>> k`;`<<` 之后 MUST 再 `>>> 0`。 |

Go 与 TS 各自 MUST 另实现一份 **BigInt 参考版**,并有测试断言快速版与参考版连续 10⁶ 次输出一致 ——
这条测试用十行代码消灭整类分块算术 bug。

### 5.4 两条流

- **建表流**:`NewPRNG(tableSeed(key))`,无 nonce,永远确定。**只有它需要跨语言一致。**
- **编码流**:用 `crypto.getRandomValues` / `crypto/rand` 播种。
  它决定选哪个编码、哪种乱序、填充位置 —— 因为解码端「组内排序后查表」,**与顺序无关**,
  所以这条流 MUST NOT 要求跨语言一致。

> 编码流随机化是必需的:否则相同答案产生相同密文,库里的「是」「5」「N/A」会彼此可关联。

---

## 6. 字节布局

### 6.1 ASCII 档(默认)

```
hint(g) = 0x40 | g ,  若结果 == 0x7F 则取 0x0A ('\n')
字母表  = {0x40..0x7E} ∪ {0x0A}          # 64 个
group(b) = b & 0x3F , 且 group(0x0A) = 0x3F
填充池  = 0x20..0x3F                      # 32 个
```

字母表需要 64 个符号,而 `0x40..0x7E` 只有 63 个,故第 64 个借用 `'\n'`。
本应用的密文只经 SQLite TEXT 与 JSON 两种载体,二者都精确保留 `'\n'`,故安全。

### 6.2 熵档

```
hint(g) = ((g & 0x30) << 1) | (g & 0x0F)
字母表  = { b : (b & 0x90) == 0 }         # 64 个,含 0x00..0x0F 控制符
group(b) = ((b >> 1) & 0x30) | (b & 0x0F)
填充池  = { 0x80+i, 0x10+i : i in 0..7 }
```

**熵档仅可用于二进制载体**,MUST NOT 经 textarea、CSV 或 QR 文本模式。

### 6.3 严格解码契约

两档下填充池与线索字母表 MUST 互不相交(有启动断言)。

解码时对每个字节:

```
if isHint(b):     收下 group(b)
elif isPadding(b): 跳过
else:             **硬报错** —— 不是「跳过」
```

上游的位掩码判定被**有意收紧**:`(b & 0x40) == 0x40` 也接受 `0x80..0xFF`,
会让任何混入的 UTF-8 文本被当作线索吞掉。精确成员判定把传输损坏变成显式失败。

---

## 7. 解码表

### 7.1 签名打包

```
packGroups(g0,g1,g2,g3):        # 先升序排序,再打包
    用 5 比较器排序网络对四个值升序排序
    return (g0 << 18) | (g1 << 12) | (g2 << 6) | g3      # 24 位,恒 < 2^31
```

排序网络(升序,固定 5 次比较):

```
if a>b: swap(a,b)
if c>d: swap(c,d)
if a>c: swap(a,c)
if b>d: swap(b,d)
if b>c: swap(b,c)
```

排序使解码键**与顺序无关** —— 这正是编码端可以自由选择 24 种乱序之一的原因。
按 6 位域打包(而非原始线缆字节)使结果恒为正,TS 侧不会看到负数。

### 7.2 建表

```
order = [0..287]
shuffle(order) 用 建表流
对 b in 0..255:
    g = order[b]
    对每个 p in 0..1819:
        if 有效(g, p):
            令 gr[i] = group(g[p[i]], p[i])       # digit = g[p[i]] - 1
            encode[b].append(gr)
            decode[packGroups(gr)] = b
```

`decode` 是**布局无关**的:同一个 group 四元组在两档下都还原成同一字节,故两档共用一张解码表。

`fingerprint = sha256( 对 b in 0..255: be16(encode[b].len) || 各四元组的 4 字节 )`。
该指纹同样**与布局无关**(它刻画的是密钥派生),用于服务器与浏览器互相证明表一致。

---

## 8. 编码

```
对每个明文字节 b:
    q    = 从 encode[b] 中随机取一个(CSPRNG 流)
    perm = 从 24 种排列中随机取一个
    对 i in 0..3:
        可选插入 pad 个填充字节, pad = nextBelow(maxPad+1)
        输出 hint(q[perm[i]])
```

随后输出 §9 尾帧。

- `maxPad` 为本应用默认 **0**:本应用以普通 JSON 走 HTTP(S),没有对抗 DPI 的需求,填充只会增大存储与带宽。
- 保留 0–100 的可配置区间,并在「编解码检视器」中可视化。

---

## 9. 完整性尾帧

body 之后追加 **4 个提示字节**,定位写出(不参与乱序、不排序),承载明文的 CRC-32(ISO-HDLC)低 24 位:

```
c = crc32(plaintext) & 0xFFFFFF
对 shift in (18, 12, 6, 0):
    输出 hint( (c >> shift) & 0x3F )
```

解码:

```
收齐全部提示字节 H
要求 len(H) >= 4 且 (len(H) - 4) mod 4 == 0        # 否则 ErrBadFrame
N = (len(H) - 4) / 4
body    = H[0 : 4N]   -> 每连续 4 个 packGroups 后查 decode
trailer = H[4N : 4N+4] -> 定位读作 24 位
校验 crc32(plaintext) & 0xFFFFFF == trailer         # 否则 ErrChecksumMismatch
```

**帧是自描述的**:明文长度由提示字节总数推出,不需要额外字段。
开销恒为 4 个符号。

---

## 10. 错误 MUST 为硬失败

```go
ErrInvalidWireByte  // 字节既非线索也不在填充池
ErrBadFrame         // 提示字节总数不满足 4N+4
ErrUnknownPuzzle    // 某个四元组不在解码表中
ErrChecksumMismatch // 校验不符
```

任何一条都不允许「尽力而为地部分解码」—— 部分解码正是静默损坏的来源。

---

## 11. 一致性保障

1. **本文件**是唯一规范来源。
2. `spec/vectors/*.json` 钉死:288 网格哈希、1820 位置组合哈希、位图哈希、
   PRNG 输出、nextBelow 边界取值、两档布局表、建表指纹、以及
   `{key, layout, maxPad, seed, 明文, 密文}` 端到端样例。
3. **双向跨语言执行**:`cmd/codec-cli`(Go)与 `frontend/scripts/codec-cli.mjs`(Node)同参数;
   对同一输入断言两侧**编码字节完全相同**(比往返更强:能抓住「双方各自自洽但都错」的分歧),
   并互相解码对方的密文。
4. **运行时自检**:两端启动时断言 288 网格、256 字节全部可编码、解码表无冲突、
   自检探针的密文等于钉死常量。失败则**拒绝启动**。
5. `make conformance` 在向量与代码不一致时失败(漂移守卫)。

> 3 的「字节完全相同」是关键:仅做往返测试无法发现两侧以各自一致但彼此不同的方式处理填充、
> 乱序或布局的分歧。

---

## 12. 安全性质(必须明确)

**本编码是混淆,不是加密。**

密钥只在一个 288 元素的数组上做置换,而 288 个网格是公开的。
知道方案的人可以观察到哪些位置组合出现,并因为 `|P_g| ∈ {12,128}` 而推断出每个字节所属的对称轨道。

因此:

- 数据库中的业务数据确实呈现为纯数独密文,且**同一答案在不同问卷中形态不同**(每问卷独立派生)。
- 但**这不提供密码学意义上的保密性**。前端 bundle 含派生密钥,拿到前端包的人可以解码。
- 需要真实保密性时,应在数独编码**之下**再叠一层 AEAD(上游隧道即如此:AEAD 负责保密,数独负责形态)。
  本项目按需求只做数独编码,故不叠。
