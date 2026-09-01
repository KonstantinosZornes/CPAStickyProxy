# 将 1024Proxy 作为账号范围的粘性会话平台接入

- **状态：** 已实现
- **日期：** 2026-08-24
- **决策范围：** StickyProxy 原生插件中的 `1024proxy` 平台适配规则；不改变 CLIProxyAPI，不引入代理中转进程，也不调用 1024Proxy 控制面 API。

---

## 背景

StickyProxy 将一个基础代理与 CPA 物理账号的规范化邮箱组合，生成确定性的最终代理 URL，并只将该 URL 写入操作员明确选中的账号 `proxy_url`。平台适配规则由同一个供应商适配器生成账号 URL 和一次性随机测试会话 URL，避免两条路径的供应商语义漂移。

1024Proxy 的动态住宅与无限住宅带宽产品公开支持 HTTP(S)、SOCKS5、账密认证、轮换会话和粘性会话。其公开文档给出以下用户名参数形态：

```text
轮换：
USERNAME-region-DE

粘性：
USERNAME-region-DE-sid-SESSION_ID-t-MINUTES
```

其中 `sid` 是会话标识，`t` 是由 1024Proxy 控制的粘性时长（动态住宅和带宽型产品公开范围为 1–120 分钟）。国家、州、省、市与 ASN 等供应商定位参数也编码在用户名中，应在改写时原样保留。本决策将 `t` 视为操作员在基础用户名中显式提供的供应商参数：StickyProxy **只管理和改写 `sid`，绝不注入、替换、删除或校验 TTL**。

操作员提供的真实 1024Proxy 动态住宅代理已验证本项目通用 URL 解析器可接受其连接结构：

```text
socks5://<username>-region-US:<password>@us.1024proxy.io:3000
```

本文不记录可用凭据、完整用户名、密码或完整可连接 URL。它们仅作为运行时代理状态保存，且页面继续只接收脱敏 URL。

**一手资料：**

- [1024Proxy 动态住宅会话控制](https://help.1024proxy.com/1024/1024proxy/dynamic-residential-traffic/session-management)
- [1024Proxy 动态住宅位置设置](https://help.1024proxy.com/1024/1024proxy/dynamic-residential-traffic/location-configuration)
- [1024Proxy 动态住宅协议](https://help.1024proxy.com/1024/1024proxy/dynamic-residential-traffic/network-protocol)
- [1024Proxy 带宽型无限住宅会话控制](https://help.1024proxy.com/1024/1024proxy/unlimited-residential-traffic-bandwidth/session-management)

---

## 决策

平台标识 `1024proxy` 已实现。它是一个**会话型供应商适配器**：基础 URL 表示用户在 1024Proxy 控制台生成的连接；对 CPA 账号同步时，StickyProxy 仅改写其中的用户名，注入由账号邮箱确定性派生的稳定供应商会话 ID（`sid`）。粘性 TTL（`t`）完全由操作员的基础 URL 和 1024Proxy 决定，插件不管理它。

其外部接口保持不变：

```go
Rewrite(proxy Proxy, email string) (RewriteResult, error)
```

调用方只需要提供已保存的代理与账号邮箱，得到可直接写入 CPA `proxy_url` 的最终 URL。供应商特有的用户名语法、旧 `sid` 替换、既有 TTL 保留和测试会话生成都封装在平台适配器内；`syncer`、账号状态识别、定向清理、回滚与状态存储不需要了解 1024Proxy 参数。

该规则已集中在一个供应商适配器模块中，而不会分散到同步调用方。四个平台共享这一私有接缝；账号改写与测试改写使用同一个适配器，避免两处平行分支发生行为漂移。

### 支持的产品范围

本次 `1024proxy` 适配器只支持以下**用户名会话参数型**代理：

- 动态住宅流量；
- 无限住宅流量（带宽型）；
- 任何未来产品，只要其控制台生成的账密代理 URL 使用可独立替换 `-sid-<id>` 的用户名语法；是否包含 `-t-<minutes>` 不影响本适配器。

不将以下产品伪装为会话型代理：

- **无限住宅流量（按端口）**：若实际端口分配是粘性来源，应该作为端口粘滞代理保留原 URL，并在得到可验证样例后单独设计；
- **长期静态 ISP**：它是已分配的长期静态 IP，不应通过随机或邮箱派生的 `sid` 重写。一个静态 IP 对应一个代理，操作员应只将其同步到其预定账号。

这一区分十分重要：稳定 `sid` 只稳定供应商会话身份；出口 IP 是否以及持续多久不变仍完全由 1024Proxy 产品与 TTL 决定。

---

## 代理接口与校验

### 平台与协议

代理记录使用：

```json
{
  "name": "US_1024_Residential",
  "platform": "1024proxy",
  "base_url": "socks5://<base-username>:<password>@us.1024proxy.io:3000"
}
```

- 平台字符串固定为小写 `1024proxy`；
- 继续使用全局直接代理 URL 接口：`http`、`https`、`socks5`，输入 `socks` 与 `socks5h` 规范化为 `socks5`；
- query 与 fragment 继续拒绝；1024Proxy 的会话参数属于用户名而非 query；
- 必须有主机和端口；
- 1024Proxy 会话型代理必须有非空用户名与非空密码。用户名是供应商认证账号和定位参数的载体；密码是上游账密认证所需的秘密。

实现不硬编码 `us.1024proxy.io:3000`、也不将主机名作为平台判断条件。控制台可能因区域、套餐或供应商变更提供不同 gateway；StickyProxy 只验证通用代理 URL 和平台所需的认证字段。实际连通性由已有服务端“测试”操作验证。

### 基础用户名不变量

基础用户名是供应商认证、定位、TTL 与可选会话模板配置。它按操作员输入的规范化 URL 原样持久化；若包含有效 `-sid-<session-id>`，该 SID 是可替换的**模板 SID**，不是 StickyProxy 为任何 CPA 账号生成或持久化的绑定。1024Proxy 适配器的唯一受管片段是一个**完整的** `-sid-<session-id>` 标记；它接受以下输入：

```text
<username-without-sid>
<username-without-sid>-sid-<session-id>
<username-without-sid>-t-<minutes>
<username-without-sid>-sid-<session-id>-t-<minutes>
```

其中：

- `<username-without-sid>` 为非空字符串，可能含 `-region-*`、`-st-*`、`-city-*`、`-asn-*` 及任何由控制台生成的供应商参数；
- `<session-id>` 必须为 `[A-Za-z0-9_-]+`；
- `-t-<minutes>` 不是 StickyProxy 的字段。其出现、位置、数值和供应商有效性均保持原样，插件不校验它，也不假设 1–120 分钟范围适用于所有未来套餐。

保存代理时，适配器只校验该模板 SID 是否可无歧义识别，**不移除、替换或归一化**它。持久化的 `base_url` 保留操作员提供的模板 SID 和所有 TTL 文本。

仅在生成账号或测试最终 URL 时，适配器才替换一个完整的 `-sid-<session-id>` 标记，或在缺少 SID 时插入新标记。若用户名以 `-sid-...-t-...` 结束，则新 `sid` 保持在既有 `-t-...` 之前；否则追加到用户名末尾。

为维持这一位置，适配器仅把**末尾** `-t-...` 识别为不透明、逐字保留的后缀。它不解释、验证、删除、添加、替换或移动其中任何 TTL 文本，也不会解析或重排供应商位置参数。

为避免对认证/定位含义做模糊猜测，含有重复 `-sid-`、空 `sid`、或无法确定为一个完整受管标记的用户名必须在保存时失败并返回稳定错误码。单独存在的 `-t-...` 绝不是错误，也不得触发归一化。持久化状态可以保存操作员提供的模板 SID 与 TTL，但从不保存账号派生的稳定 hash 或测试随机 SID；每次账号同步或测试时才生成并注入对应 SID。

---

## 账号 URL 改写

对每个同步目标，沿用项目既有稳定身份算法：

```text
normalized_email = lowercase(trim(email))
stable_hash = lowercase_hex(SHA-256("stickyproxy:v1" + "\\x1f" + normalized_email))[0:24]
```

1024Proxy 适配器将 username 中的现有 `sid` 替换为稳定 hash；若不存在 `sid`，则注入一个。除此之外，用户名逐字保留：

```text
基础用户名中没有 sid：
<base-username>
→ <base-username>-sid-<stable_hash>

基础用户名中包含 sid 和操作员 TTL：
<base-username>-sid-previous-t-60
→ <base-username>-sid-<stable_hash>-t-60
```

不注入固定 TTL，也不从基础 URL 移除 TTL。无 `-t-...` 的基础用户名保持无 TTL；已有的 `-t-...` 保留其原始数值和位置。因而代理粘性时长是操作员在 1024Proxy 控制台/基础 URL 中作出的显式选择，不是 StickyProxy 为账号暗中施加的策略。

示例（所有敏感字段为示意）：

```text
基础用户名：
<base-user>-region-US

账号生效用户名：
<base-user>-region-US-sid-f2783dad6a985ee016173293

带操作员选择的 TTL 的基础用户名：
<base-user>-region-US-t-60

账号生效用户名：
<base-user>-region-US-sid-f2783dad6a985ee016173293-t-60
```

最终 URL 继续通过 `domain.Build` 构造；它会正确编码 URL 用户名/密码中的保留字符。浏览器只获取 `domain.Mask` 后的脱敏最终 URL。

### 生命周期语义

- 相同账号 + 相同代理始终生成相同 URL，因此重复应用跳过与按代理定向清理可工作；
- 同一账号重新应用另一代理后保持相同 `sid`，但 gateway、凭据、定位参数或出口池可能不同；
- `sid` 不是密码，也不是永久 IP 分配；1024Proxy 会话或出口 IP 仍会按操作员提供的 TTL（若有）或供应商套餐默认行为改变；
- 一次成功应用会将账号 `auth_index` 到该代理 `proxy_id` 的映射与 CPA `proxy_url` 一同持久化；映射不保存最终 URL、邮箱或稳定 SID；
- 按代理清除时，只清理本次选中且账号代理映射指向该代理的账号；不读取或比较其他账号的 `proxy_url`。

---

## 服务端测试

代理测试必须证明“此基础代理当前能够通过上游代理访问”，但绝不能占用、泄露或复用任一 CPA 账号的稳定会话。

每次测试：

1. 生成加密随机的 `test<24-hex>` 会话 ID；
2. 仅替换或注入 `<base-username>-sid-test<random>`，并逐字保留基础用户名中已有的 `-t-...` 或其他供应商参数；
3. 使用已有 15 秒服务端超时和 `ipwho.is` 出口查询；
4. 只向页面返回平台、出口 IP 与地理信息，不返回完整代理 URL、密码、基础用户名或随机会话 ID。

测试不得为了缩短会话而注入 `-t-5`，也不得删除或修改操作员选择的 TTL。随机 ID 与稳定账号 hash 的命名空间不同，因此测试 URL 永远不会等于真实账号 URL。

---

## 模块设计与实现

### 已实现的接缝

在 `plugin/internal/domain` 内建立私有的供应商适配器接缝。公共模块接口仍然是代理构造、账号 URL 改写、脱敏和测试 URL 生成；上层不接触任何供应商语法。

内部职责：

```go
// providerAdapter 负责单个供应商的凭据校验、基础用户名规范化、
// 账号会话改写和隔离测试会话改写。
type providerAdapter interface {
    Validate(parts Parts) error
    NormalizeBase(parts Parts) (Parts, error)
    RewriteAccount(parts Parts, stableHash string) (RewriteResult, error)
    RewriteTest(parts Parts, randomSession string) (RewriteResult, error)
}
```

该接缝有四个适配器（Decodo、DataImpulse、Resin、1024Proxy），以一个小接口封装四种语法：

- `NewProxy` 查找适配器，执行认证校验和 base URL 规范化；
- `Rewrite` 查找同一适配器并生成账号 URL；
- `tester.randomSessionProxy` 通过公开受限的 domain 测试 URL helper 使用同一适配器；
- `syncer`、`app`、`state`、前端请求格式都不承担供应商分支。

公共 `domain.Rewrite` 接口保持不变；1024Proxy 字符串操作不在两处平行 `switch` 中复制。后续平台新增、旧会话清理与测试隔离因此保持**局部性**，并可通过同一适配器接口测试。

### 已完成的实现范围

| 位置 | 变更 |
|---|---|
| `plugin/internal/domain/proxy.go` 或拆出的同包 provider 文件 | 增加 `Proxy1024 = "1024proxy"`、平台注册、仅替换/注入 `sid` 的用户名解析与共享测试 URL helper；任何 `-t-...` 文本必须透传。 |
| `plugin/internal/tester/tester.go` | 删除/收敛平台专用测试 URL 分支，调用 domain 的隔离随机测试会话 helper；不得改变基础 URL 的 TTL。 |
| `plugin/internal/i18n/messages.go` | 增加不泄露原始 URL 的稳定错误码，例如缺少密码、无效或重复的 1024 `sid` 标记；不增加 TTL 校验错误。将 `proxy_static` 改为平台中性的表述，或在确实支持端口型 1024 前保持该状态只用于 Decodo。 |
| `plugin/web-ui/src/types.ts` | `ProxyPlatform` 联合加入 `"1024proxy"`。 |
| `plugin/web-ui/src/components/ProxyForm.tsx` | 平台选择中加入 1024Proxy。 |
| `plugin/web-ui/src/components/BulkProxyForm.tsx` | 批量导入平台选择中加入 1024Proxy。 |
| `plugin/web-ui/src/components/ProxyList.tsx` 与样式 | 加入明确的 1024Proxy 标记，避免落入 Decodo 默认标识。 |
| 中英文前端词典 | 增加 1024Proxy 名称与新稳定错误码翻译。 |
| `README.md`、`CONTEXT.md`、主设计文档 | 实现后更新支持平台、会话语义和使用示例；不得将实际凭据写入文档。 |

### 已验证的测试契约

以下测试已通过；测试样例一律使用虚构 gateway、虚构认证和无效示例凭据：

1. `NormalizePlatform("1024proxy")` 成功，未知名称仍失败；
2. 会话型 1024Proxy URL 缺用户名或密码时保存失败；
3. `socks5://<base>-region-US:<password>@example:3000` 规范化后保留协议、gateway、认证和 `-region-US`；
4. 粘贴已有 `-sid-old-t-60` 的 URL 后，持久化的 base URL 精确保留模板 `-sid-old-t-60`；账号和测试最终 URL 才将其中 `sid` 替换为各自生成的值；
5. 同一 email 重写两次得到完全相同最终 URL，且 username 精确包含一个 `-sid-<stable-hash>`；
6. 无 TTL 的基础用户名在同步/测试后仍无 TTL；含 `-t-60` 的基础用户名在同步/测试后仍精确含 `-t-60`，不得注入、替换或删除任何 `-t-...`；
7. 不同 email 的最终 URL 使用不同 `sid`，且最终 URL 不包含原始邮箱；
8. 不合法/重复/非尾部 `sid` 标记以稳定错误失败，不做模糊文本删除；单独存在的 TTL 不得失败；
9. 每次测试使用不同 `test<random>` ID，且永不使用账号 hash 或改变基础 URL 的 TTL；
10. 对同一账号重复应用同一 1024Proxy 代理时识别为已应用；按代理清除只清理本次选中且匹配该 1024Proxy 代理预期 URL 的账号；
11. 管理 API 预览、账号列表与测试响应只展示脱敏 URL，测试响应不含认证或 session ID；
12. 前端类型检查、单元测试，以及 Go 全量测试继续通过。

---

## 替代方案与取舍

### 1. 将 1024Proxy 当作普通/原样代理

**拒绝。** 原样代理会保留轮换行为，无法为每个 CPA 账号生成不同的会话标识；同一个上游账号在并发时会混合多个 CPA 身份，也无法由当前的确定性 URL 比较安全地识别和清理。

### 2. 将稳定 hash 写入密码、query 或额外状态文件

**拒绝。** 官方公开会话语法位于用户名，query 已被通用 URL 接口拒绝；密码应保持为供应商秘密。额外状态文件会破坏“最终 URL 可由代理 + 邮箱确定性重建”的设计，并给恢复、清理和隐私带来额外状态。

### 3. 通过 1024Proxy API 分配和维护会话/IP

**拒绝（本次）。** 这会引入 token 管理、网络失败状态、租约回收、IP 映射持久化和额外控制面依赖，显著扩大接口。用户名会话适配器已覆盖当前目标，且不会启动额外进程或修改 CPA。

### 4. 由 StickyProxy 注入或管理每个代理的 TTL

**拒绝。** TTL 是供应商会话策略，不是账号身份。操作员应在 1024Proxy 控制台生成的基础用户名中显式选择它；StickyProxy 对基础用户名中存在的 `-t-...` 保持透明透传。因而不增加 `session_ttl_minutes` 状态字段、编辑 UI、迁移或 TTL 校验契约。

### 5. 为端口型和长期静态 ISP 复用 `1024proxy` 会话适配器

**拒绝。** 它们的粘滞来源分别可能是端口分配与已购静态 IP，而不是 `sid`。混用会错误宣称账号 hash 能控制 IP。若需要支持，应在拿到可验证控制台样例后增加独立平台/模式与明确的 `PortStickyOnly` 或静态分配语义。

---

## 后果

### 正向后果

- 已选 CPA 账号可以在 1024Proxy 的公开会话语法内使用稳定且互不相同的粘滞身份；
- 现有账号选择、同步回滚、状态识别、定向清理、URL 脱敏和服务端测试继续复用；
- 没有新的 CPA 修改、浏览器秘密传递、代理桥接服务或上游 API token；
- 供应商规则的复杂性集中在一个深模块中，新增供应商不再要求维护两个易漂移的平行分支。

### 限制与风险

- 出口 IP 可能按操作员在基础 URL 中选择的 TTL，或按供应商套餐默认行为变化；它不是长期静态 IP 方案；
- 供应商可能对 会话 ID 字符集、最大长度、参数顺序或不同套餐行为有额外未公开限制；首次实现必须用控制台真实代理执行服务端“测试”与一个非生产 CPA 账号的同步验证；
- 如果未来控制台生成的 URL 表明 会话语法与公开文档不同，实现必须以该实际产品的官方控制台/支持确认内容修订本 ADR 后再放宽解析规则；
- 基础 URL 当前按项目既有策略明文持久化，可能包含密码；状态文件权限和页面脱敏约束仍是保护边界，不能将实际凭据加入 Git、测试夹具、截图或文档。

---

## 验收结果

本决策已满足下列验收条件：

1. 页面可新增、编辑和批量导入 `1024proxy` 会话型代理；
2. 对提供的 SOCKS5 代理，服务端测试可在不向浏览器泄露凭据的情况下返回出口信息；
3. 同一选定 CPA 账号重复同步时生成的 URL 一致，不同账号使用不同会话 ID；
4. 取消当前代理和显式清除操作维持现有选择性、定向和回滚语义；
5. 端口型与长期静态 ISP 不被错误注入 `sid`；
6. Go 和 Web 既有验证命令及新增 1024Proxy 测试全部通过。
