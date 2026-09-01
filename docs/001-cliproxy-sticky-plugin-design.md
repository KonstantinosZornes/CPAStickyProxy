# StickyProxy / 粘滞代理设计

> **状态：** V2 已实现并部署；V3“账号级独立应用代理”已确定为后续目标模型。
> **插件：** `stickyproxy.so` / `stickyproxy.dylib` / `stickyproxy.dll`
> **目标：** 操作员从任意已保存代理发起“应用代理”，按 CPA 账号邮箱生成稳定身份，改写 Decodo、DataImpulse、Resin 或 1024Proxy 的代理 URL，并只写入明确选中的账号。

---

## 1. 设计变更

### V1：早期行为

```text
选择代理
→ 同步全部带邮箱账号
→ 覆盖全部账号的 proxy_url
```

### V2：已部署的选择性同步

```text
当前代理
→ 打开账号筛选/勾选页
→ 仅写入选中账号
```

V2 消除了自动全量同步，但仍保留“当前代理 / 切换并同步”的全局操作语义。

### V3：账号级独立应用代理（目标模型）

```text
从代理 A 点击「应用代理」
→ 选择账号 Alice、Bob
→ 仅为 Alice、Bob 写入代理 A 的最终 proxy_url

从代理 B 点击「应用代理」
→ 选择账号 Carol
→ 仅为 Carol 写入代理 B 的最终 proxy_url

再次把代理 C 应用给 Alice
→ 仅覆盖 Alice 原有 proxy_url
→ Bob 和 Carol 不受影响
```

这意味着：

- 不存在唯一“当前代理”，也没有“切换代理”操作；
- 每条代理都可独立应用到零个或多个账号；
- 一个账号同一时刻最多使用一个账号级 `proxy_url`；再次应用另一代理只覆盖该账号；
- 关闭账号选择窗口不会修改任何账号；
- 未选账号始终保留既有 `proxy_url`；
- 页面选择结果只服务当前一次操作；成功应用后，插件会持久化该账号到代理的映射；
- 每个账号仍按邮箱得到稳定 hash；同一账号再次应用同一代理时，最终 URL 保持一致。

`active_proxy_id`、切换、取消当前代理和依赖当前代理的状态标签属于 V2 遗留概念，在 V3 实现时移除。

---

## 2. 核心模型

```text
发起操作的代理
+ 已选择账号的规范化邮箱
= stable_hash
= 平台改写后的账号 proxy_url
```

### 2.1 术语

| 名称 | 定义 |
|---|---|
| **代理** | 有名称、平台和基础地址的 HTTP/HTTPS/SOCKS5 代理配置。 |
| **应用代理** | 操作员从某条代理发起，对明确选中账号写入该代理最终代理 URL 的操作。 |
| **应用目标** | 本次“应用代理”页面里操作员显式勾选的 CPA 物理认证账号。 |
| **稳定 hash** | 仅由规范化邮箱生成的 SHA-256 截断值；不持久化、不随机。 |
| **账号代理字段** | CPA 既有的 `proxy_url`；它优先于全局 `proxy-url`。 |
| **账号代理映射** | StickyProxy 状态中以 `auth_index` 为键、以 `proxy_id` 为值的持久化记录。它是账号已应用哪条代理的唯一来源。 |
| **账号已应用代理** | 账号代理映射指向一条当前已保存的代理；页面显示该代理名称。它不证明账号实际代理连通，也不通过 `proxy_url` 反查。 |
| **应用快照** | 发起操作的代理 ID、账号筛选和选择组成的一次性页面状态；操作结束即丢弃。 |

### 2.2 插件状态

V3 插件状态保存代理和账号代理映射：

```json
{
  "proxies": [
    {
      "name": "US Residential",
      "platform": "decodo",
      "base_url": "http://login:password@gate.decodo.com:7000"
    }
  ],
  "account_proxy_mappings": [
    {
      "auth_index": "codex-alice.json",
      "proxy_id": "px_0123456789abcdef"
    }
  ]
}
```

该格式不兼容旧状态字段。升级前必须备份旧状态文件；升级后重新添加所需代理并重新应用给账号。

不保存：

```text
账号邮箱列表
账号选择结果
最终 URL
稳定 hash
账号 SID
session / sessid
同步历史
使用次数
IP 历史
```

账号实际代理仍保存在 CPA 已有认证文件的 `proxy_url` 中；这是 CPA 本身的字段，不是 StickyProxy 新建的绑定表。

---

## 3. 稳定身份与平台规则

### 3.1 邮箱 hash

```text
normalized_email = lowercase(trim(email))
hash_input = "stickyproxy:v1" + "\x1f" + normalized_email
stable_hash = lowercase_hex(SHA-256(hash_input))[0:24]
```

示例：

```text
alice@example.com
→ f2783dad6a985ee016173293
```

- 同一邮箱总是相同；
- 不同邮箱不同；
- 对同一账号应用不同代理不改变 hash，只改变所使用的基础代理；
- hash 不是保密值。

### 3.2 支持协议

```text
http://[user:password@]host:port
https://[user:password@]host:port
socks5://[user:password@]host:port
```

兼容输入：`socks://`、`socks5h://`。其他协议拒绝。

### 3.3 DataImpulse

```text
base:
http://login__cr.us:password@gw.dataimpulse.com:10000

effective:
http://login__cr.us;sessid.<stable_hash>:password@gw.dataimpulse.com:10000
```

### 3.4 Decodo

仅 `gate.decodo.com` 注入用户名 session：

```text
base:
http://login:password@gate.decodo.com:7000

effective:
http://user-login-session-<stable_hash>-sessionduration-30:password@gate.decodo.com:7000
```

`isp.decodo.com`、`dc.decodo.com` 等端口型粘滞地址保留原 URL，不注入 session。

### 3.5 Resin

```text
base:
socks5://ignored:RESIN_PROXY_TOKEN@resin.example:2260

effective:
socks5://Default.sp_<stable_hash>:RESIN_PROXY_TOKEN@resin.example:2260
```

Resin 以 `[Default, sp_<stable_hash>]` 维护自身的 sticky lease。

### 3.6 1024Proxy

1024Proxy 支持 session-username 代理。代理必须有非空用户名和密码；主机与端口由操作员控制台提供，不按特定 gateway 硬编码。账号 URL 只管理用户名中一个完整的 `-sid-<id>` 标记：已有模板 SID 被替换，缺少 SID 时插入。稳定 SID 仍使用本节的邮箱派生 `stable_hash`，因此同一账号在同一代理上重复同步得到相同 URL。

```text
base:
socks5://<base-user>-region-US-t-60:<password>@gateway.example:3000

effective:
socks5://<base-user>-region-US-sid-<stable_hash>-t-60:<password>@gateway.example:3000
```

`-t-...` 是操作员提供的供应商 TTL，不是 StickyProxy 字段。它和其他用户名定位参数均逐字保留：插件不解释、校验、注入、删除、替换或重排 TTL。模板 SID 与 TTL 作为基础 URL 原文保存；账号稳定 SID 与测试随机 SID 都不保存。

---

## 4. 账号选择与筛选页面

### 4.1 唯一的入口：从代理应用

每条代理都有独立操作：

```text
应用代理
```

点击某条代理的“应用代理”后，打开唯一的账号选择窗口；该窗口明确显示发起操作的代理。点击操作本身不立即写入任何账号。

```text
代理 A → 应用代理 → 选择账号 → 仅将 A 写入所选账号
代理 B → 应用代理 → 选择账号 → 仅将 B 写入所选账号
```

没有“当前代理”“切换并同步”或独立的“账号代理配置”页签。账号查看、筛选、全选、应用和清除均复用这一个窗口。

### 4.2 应用代理窗口结构

```text
┌────────────────────────────────────────────────────────────────────┐
│ 应用代理                                                    [关闭] │
│ 本次代理：homeus · DataImpulse                                      │
│ 仅选中的账号会应用此代理的粘滞代理。                            │
├────────────────────────────────────────────────────────────────────┤
│ 搜索 [邮箱 / 名称 / 标签________________]                         │
│ Provider [全部 ▼]   类型 [全部 ▼]   状态 [全部 ▼]                 │
│ 优先级 [全部 ▼]     备注 [包含文本____________]                   │
│                                                                    │
│ [全选当前筛选结果] [全不选]              已选 0 / 356             │
├────────────────────────────────────────────────────────────────────┤
│ □  alice@example.com   Codex   OAuth   active   priority: 0        │
│    alice.json · 已应用 StickyProxy 代理 · homeus                  │
│ □  bob@example.com     Claude  OAuth   unavailable                 │
│    bob.json · 继承系统代理                                        │
│ □  ...                                                            │
├────────────────────────────────────────────────────────────────────┤
│ 第 1 页；符合条件账号可继续翻页。                 [上一页] [下一页] │
│                                                                    │
│ [取消]                                    [应用代理到 0 个账号]   │
└────────────────────────────────────────────────────────────────────┘
```

### 4.3 账号代理状态

窗口不比较某个全局当前代理。它只查询持久化账号代理映射：映射指向当前保存的代理时显示该代理名称；没有有效映射时，才读取账号 `proxy_url` 区分继承系统代理、其他账号代理与格式无效。它不从 URL 反查代理，也不生成账号最终 URL。

| 状态值 | 页面标签 | 判定 | 应用含义 |
|---|---|---|---|
| `sticky_applied` | 已应用 StickyProxy 代理 · `<代理名>` | 账号代理映射指向一条当前保存的代理。 | 可再次应用任意代理；若选同一代理则计入 `already_current`。 |
| `inherits_system` | 继承系统代理 | 没有有效映射且账号 `proxy_url` 为空。 | 可选；应用后写入本次代理。 |
| `custom_other` | 其他账号代理 | 没有有效映射且 `proxy_url` 非空、可解析。 | 可选；确认后会被本次代理覆盖。 |
| `no_email` | 无邮箱 | 物理账号没有可规范化 email。 | 不可选；不能派生稳定身份。 |
| `runtime_only` | 运行时账号 | CPA 账号没有可写的物理认证文件。 | 不可选；不写入。 |
| `invalid_proxy` | 账号代理格式无效 | 没有有效映射且 `proxy_url` 非空但无法解析为支持格式。 | 可选；应用会用本次代理覆盖。 |

说明：

```text
sticky_applied 只表示插件持久化映射指向某条保存代理；
它不证明代理可连通、账号 proxy_url 未被外部修改或出口 IP 未变化。
```

代理的“测试”按钮仍用于验证代理本身的随机出口 IP/国家；如需检查某个账号真实粘滞出口，可后续单独增加“按账号测试”。

### 4.4 页面筛选与选择

唯一账号选择窗口使用以下筛选字段：

```text
关键字（email/name/label/note）
Provider
认证类型
CPA 状态
启用/禁用
优先级范围
备注
代理状态（sticky_applied / inherits_system / custom_other / no_email / runtime_only / invalid_proxy）
```

支持：

```text
全选当前筛选结果
全不选
跨页手动勾选
应用代理到选中账号
清除选中账号代理
按代理清除选中账号
```

默认：

```text
显示每个账号已应用的代理（如可识别）
没有默认勾选
```

### 4.4 可展示的账号属性

账号列表来自 CPA 的 `host.auth.list`，只返回已经可公开在 CPA 管理界面显示的脱敏元数据：

```json
{
  "auth_index": "ae6db424ddea6d02",
  "name": "alice.json",
  "email": "alice@example.com",
  "label": "alice@example.com",
  "provider": "codex",
  "type": "oauth",
  "account_type": "oauth",
  "status": "active",
  "disabled": false,
  "unavailable": false,
  "priority": 0,
  "note": "production"
}
```

绝不返回：

```text
proxy_url 原文
基础代理地址
代理密码
access_token
refresh_token
cookie
认证 JSON
```

### 4.5 筛选条件

支持按以下属性组合过滤：

| 筛选项 | 行为 |
|---|---|
| 关键字 | 模糊匹配 `email`、`name`、`label`、`note`。 |
| Provider | 由全部 CPA 物理账号的实际 `provider` 值动态汇总为下拉项，例如 Codex、Claude、Gemini、Antigravity。 |
| 类型 | 由全部 CPA 物理账号的 `type`（缺省时 `account_type`）动态汇总为下拉项。 |
| 状态 | 由全部 CPA 物理账号的实际 `status` 值动态汇总为下拉项。 |
| 启用状态 | 全部、仅启用、仅禁用。 |
| 优先级 | 由全部 CPA 物理账号的实际优先级动态汇总、去重并升序排列为下拉项。 |
| 备注 | 匹配 CPA 账号 `note`。 |

动态下拉 facets 与分页结果一起返回，但汇总范围始终是**全部物理账号**，不受当前页、关键字、代理状态或其他筛选条件影响；因此操作员可以稳定调整或清除筛选项。

默认筛选：

```text
status = active
include disabled = false
```

操作员可以显式放宽筛选，把 disabled/unavailable 账号包含进同步目标。

### 4.6 勾选语义

- 初次打开时，**默认不勾选任何账号**；
- `全选当前筛选结果`：浏览器保存筛选快照；确认应用时由服务端重新找出匹配账号，不向浏览器传输全部账号 ID；
- `全不选`：清空全部已选账号；
- 手动勾选跨分页保持；
- 全选后手动取消的账号保存为本次筛选快照的排除项；
- 改任意筛选条件、关闭窗口、刷新页面或完成操作时，当前选择集丢弃；
- 操作文案随模式变化：

```text
应用代理到 12 个账号
清除选中账号代理
按此代理清除 12 个账号
```

- 未选择账号时按钮禁用。

### 4.7 分页和上限

账号列表服务端分页：

```text
page_size 默认 50
最大 100
```

普通元数据筛选返回精确总数。代理状态筛选需要读取账号 `proxy_url`，采用渐进分页：页面显示是否还有下一页，不为了显示总数扫描全部账号配置。

一次应用或清除最多处理 2,000 个最终可操作账号。筛选结果达到第 2,001 个时，服务端返回 `selection_too_large`，且不修改任何账号。

---

## 5. 管理 API 设计

所有接口仍在 CPA 的 `/v0/management` 鉴权之后调用。

### 5.1 查询账号代理状态

```text
GET /v0/management/stickyproxy/accounts
```

Query：

```text
q=alice
provider=codex,claude
type=oauth
status=active
include_disabled=false
priority_min=0
priority_max=10
note=production
proxy_state=sticky_applied,inherits_system
page=1
page_size=50
```

响应中的每个账号可返回脱敏状态和匹配代理名称：

```json
{
  "ok": true,
  "total_known": true,
  "total": 356,
  "page": 1,
  "page_size": 50,
  "has_more": true,
  "items": [
    {
      "auth_index": "ae6db424ddea6d02",
      "email": "alice@example.com",
      "provider": "codex",
      "status": "active",
      "proxy_state": "sticky_applied",
      "proxy_name": "homeus"
    }
  ]
}
```

### 5.2 应用代理到选中账号

```text
POST /v0/management/stickyproxy/apply
```

请求必须带发起操作的 `proxy_id` 和一种选择方式：显式账号 ID，或紧凑的筛选快照与排除项。

```json
{
  "proxy_id": "px_72df05d20ad58ad8",
  "selection": {
    "kind": "filtered",
    "filters": {
      "provider": "codex",
      "status": "active"
    },
    "excluded_auth_indexes": ["a1b2c3d4e5f60708"]
  }
}
```

规则：

1. `proxy_id` 必须仍存在；代理被编辑或删除后，旧窗口请求返回稳定错误并要求重新打开；
2. 服务端只处理本次选择最终解析出的账号，最多 2,000 个；
3. 每个账号按邮箱和**本次代理**计算最终 URL；
4. 只写入本次账号的 `proxy_url`；再次应用另一代理时，只覆盖该账号；
5. 未选账号的 `proxy_url` 不变；
6. 对本次账号执行补偿事务：任一写入失败时回滚本次已写入账号；
7. 不返回完整 URL、代理密码、随机会话或认证 JSON。

响应：

```json
{
  "ok": true,
  "proxy_id": "px_72df05d20ad58ad8",
  "selected": 12,
  "eligible": 12,
  "updated": 10,
  "already_applied": 2,
  "skipped": 0,
  "errors": []
}
```

### 5.3 清除账号代理

通用清除不关心账号此前使用哪条代理：

```text
POST /v0/management/stickyproxy/clear-proxy
```

```json
{
  "selection": {
    "kind": "explicit",
    "auth_indexes": ["..."]
  }
}
```

对本次账号设置：

```json
{ "proxy_url": "" }
```

### 5.4 按代理清除

每条代理提供“清除使用此代理的账号代理”。它打开与应用代理相同的账号选择窗口，并要求明确的 `proxy_id` 与账号选择：

```text
POST /v0/management/stickyproxy/clear-applied
```

服务端仅清空同时满足下列条件的账号：

```text
账号被本次选中
且账号代理映射的 proxy_id 等于本次代理 ID
```

因此，不会清除选中账号上的人工代理、其他 StickyProxy 代理或不匹配地址。不存在“取消当前代理”这种隐式全局清理。

### 5.5 代理随机出口测试

每个代理继续使用：

```text
POST /v0/management/stickyproxy/proxies/test
{ "proxy_id": "px_..." }
```

服务端每次生成新的随机供应商会话：

```text
DataImpulse -> ;sessid.test<24 random hex>
Decodo      -> -session-test<24 random hex>-sessionduration-30
Resin       -> Default.test_test<24 random hex>
1024Proxy   -> -sid-test<24 random hex>（保留基础用户名中所有 `-t-...` 文本）
```

返回：

```json
{
  "ok": true,
  "test": {
    "platform": "dataimpulse",
    "ip": "72.56.164.114",
    "country": "United States",
    "country_code": "US",
    "region": "New York",
    "city": "New York City",
    "port_sticky_only": false
  }
}
```

不会返回基础地址、密码、Resin Token、1024Proxy 基础用户名、随机会话 ID 或完整代理 URL。1024Proxy 测试 SID 与账号稳定 SID 使用不同命名空间，测试不会复用或占用账号会话。

---

## 6. CPA 主页面认证与主题

插件 Resource：

```text
/v0/resource/plugins/stickyproxy/dashboard
```

仍是公开静态 shell；敏感数据和写操作只走 `/v0/management/stickyproxy/...`。

当 Resource 与 CPA Management Center 同源时，页面读取既有的 Zustand 持久化认证和语言状态：

```text
cli-proxy-auth
cli-proxy-language
```

兼容 CPA 的 `enc::v1::` 轻度混淆认证格式，使用：

```text
apiBase
managementKey
language
```

页面不会：

```text
显示第二次 token 输入框
写入新的认证 localStorage
将 token 放入 URL / cookie / postMessage
依赖 Nginx 注入或改写页面
```

若 Resource 与主面板跨 origin，浏览器同源策略禁止插件读取主面板的 localStorage。主面板主题继承已从当前版本移出，后续将通过 CPA 主面板自身提供的协议单独设计；当前页面使用自身同源偏好或浏览器颜色方案回退。

若无法读取 CPA 主页面认证，页面只显示：

```text
请先在 CPA 管理中心登录
```

---

## 7. V3 页面设计

```text
┌──────────────────────────────────────────────────────────────────┐
│ StickyProxy / 粘滞代理                         [中文 / English] │
├──────────────────────────────────────────────────────────────────┤
│ 代理列表                                      [批量] [添加] │
│                                                                    │
│   homeus      DataImpulse [应用代理] [按代理清除] [测试] [编辑] │
│   http://...:***@gw.dataimpulse.com:10000                         │
│                                                                    │
│   datacenter  DataImpulse [应用代理] [按代理清除] [测试] [编辑] │
│   http://...:***@dc.example:10000                                 │
├──────────────────────────────────────────────────────────────────┤
│ 按邮箱预览                                                        │
│ 选择代理 [homeus ▼]  Email [alice@example.com____] [预览]    │
└──────────────────────────────────────────────────────────────────┘
```

每条代理的“应用代理”都打开同一个账号选择窗口；代理只是本次操作输入，不会成为全局当前项。

视觉使用 StickyProxy 自身维护的 CPA 风格暖灰变量、卡片、边框和按钮圆角。跨 origin 的主面板主题继承不在当前版本实现，也不依赖 Nginx；后续单独设计 CPA 主面板协议。

---

## 8. Native C ABI

插件导出：

```c
int  cliproxy_plugin_init(const cliproxy_host_api*, cliproxy_plugin_api*);
int  cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
void cliproxyPluginFree(void*, size_t);
void cliproxyPluginShutdown(void);
```

- host ABI version：`1`；
- plugin ABI version：`1`；
- RPC schema version：`3`（与当前 CPA 插件宿主兼容）；
- panic 内容不直接返回，避免密码泄露；
- Resource 精确匹配 `/v0/resource/plugins/stickyproxy/dashboard`。

---

## 9. V3 验收标准

1. 不存在唯一当前代理、切换代理、取消当前代理或 `active_proxy_id` 产品语义；
2. 每个代理均可打开同一个“应用代理”账号选择窗口；
3. 账号选择窗口能按邮箱、名称、Provider、类型、状态、优先级、备注和账号代理状态过滤；
4. 支持全选当前筛选结果、全不选、跨分页手动勾选和筛选快照排除项；
5. 默认未选中任何账号；
6. 应用请求只修改本次最终解析出的账号；未选账号的 `proxy_url` 不变；
7. 同一账号应用另一代理时，只覆盖该账号；其他账号继续使用原代理；
8. 账号页面能显示已识别的 StickyProxy 代理名称、继承系统代理或其他账号代理；
9. 按代理清除只清理本次选中且实际使用该代理的账号；通用清除只清理本次选中账号；
10. 应用或清除超过 2,000 个最终可操作账号时零写入；任一写入失败回滚本次已修改账号；
11. 后端失败响应和操作明细只返回稳定 `code`（与可选 `auth_index`），前端根据本地中英文词典翻译；账号列表、操作响应、测试响应不泄露完整 proxy URL、密码、Token 或认证 JSON；
12. 每个代理测试都使用新的随机供应商会话，返回 IP、国家、地区、城市；
13. 同源运行时页面继承 CPA 主页面认证，不再要求第二次输入管理令牌；跨 origin 主题继承后续单独设计，且不依赖 Nginx；
14. React + TypeScript 源码位于 `plugin/web-ui/src`，只将 Vite 单文件产物嵌入插件。
