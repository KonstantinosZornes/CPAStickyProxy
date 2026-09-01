# 增加普通代理平台设计

- **状态：** 已实现
- **日期：** 2026-08-24
- **范围：** 增加一种可应用到指定账号、但不会帮每个账号固定会话的代理。

## 背景

现有平台会按账号邮箱生成稳定身份，再按供应商规则改写代理 URL。

有些普通 HTTP、HTTPS 或 SOCKS5 代理没有可安全改写的会话语法。把它们伪装成现有供应商会改坏认证信息，也会错误地让人以为插件能保证粘滞出口。

因此新增普通代理平台：它只负责把你填写的代理地址应用给选中的账号，不改用户名或密码，也不管理上游会话或出口 IP。

## 平台规则

平台值为 `generic`，页面显示为“普通代理”。

代理结构不变：

```json
{
  "name": "office_egress",
  "platform": "generic",
  "base_url": "socks5://operator:password@gateway.example:1080"
}
```

普通代理只接受已有的直接代理格式：

```text
http://[username:password@]host:port
https://[username:password@]host:port
socks5://[username:password@]host:port
```

- 输入兼容 `socks://` 与 `socks5h://`，保存时统一为 `socks5://`。
- 主机和端口必填；query、fragment 和其他协议继续拒绝。
- 用户名和密码都可选。
- 保存时只统一地址写法，例如将 `socks://` 和 `socks5h://` 写为 `socks5://`、将主机名改为小写；不会改写用户名、密码、TTL 或其他供应商参数。

同一代理地址保存为 `generic` 和保存为某个供应商平台时，是两个不同代理，因为后者可能会改写认证信息，普通代理不会。

## 最终代理地址

普通代理不使用账号邮箱，不生成 hash、session、SID 或 TTL。它会把保存后的代理地址直接交给账号：

```text
最终代理地址 = 保存后的代理地址
```

例如，两个账号应用同一代理后得到相同地址：

```text
alice@example.com -> socks5://operator:password@gateway.example:1080
bob@example.com   -> socks5://operator:password@gateway.example:1080
```

这是预期行为。插件不保证这些账号的出口 IP 相同、不同、稳定或可用；这些行为由上游代理决定。

## 应用与清除

普通代理沿用现有的账号选择和一次最多 2,000 个可操作账号的规则：

1. 操作员从普通代理条目点击“应用代理”。
2. 选择要修改的账号。
3. 插件将保存后的代理地址写入这些账号的 `proxy_url`，并为成功处理的账号持久化 `{ auth_index, proxy_id }` 映射。
4. 未选账号不变；同一账号再次应用其他代理时，只覆盖该账号的 `proxy_url` 与代理映射。
5. 某个账号读写失败时，记录该账号错误并继续处理其他账号；只有实际成功处理的账号更新映射。

重复把同一普通代理应用给同一账号时，地址不变，应计为 `already_applied`，而不是重复写入。

本次不改变账号资格：无邮箱、运行时、不可写或损坏的账号仍不能被选择。普通代理虽然不依赖邮箱，但支持这类账号需要单独设计。

账号状态识别只查询持久化账号代理映射：映射指向该普通代理时显示为已应用。相同地址即使原本由人工写入，也不会被识别为该代理；这只表示插件不推断来源或可用性。

命中的账号显示“已应用代理 · `<代理名>`”，不能显示为“粘滞代理”。

“按代理清除”只清空本次明确选中且账号代理映射指向该普通代理的账号。它不会清除未选账号、映射到其他代理的账号或未映射账号。删除代理会删除关联映射，但不会修改任何账号的 `proxy_url`。

## 测试

测试直接使用保存后的普通代理地址查询出口 IP：

```text
test_url = 保存后的代理地址
```

测试不会生成随机 session，也不使用账号 hash。响应仍只返回平台、出口 IP 和地理信息，不返回完整 URL、用户名或密码。

普通代理没有由插件控制的测试会话隔离。测试只表示该地址当前的出口情况，不保证与账号未来请求的出口一致。

## 实现范围

在 `plugin/internal/domain` 既有的 provider Adapter 中增加 `genericAdapter`：

- 校验只依赖通用 URL 解析器；
- 账号 URL 和测试 URL 都直接返回保存后的代理地址；
- 不设置 `StableHash`，不使用 `PortStickyOnly`。

前端的 `ProxyPlatform` 类型、代理表单、批量导入、代理标签和中英文词典加入 `generic`。现有代理、账号选择、应用、清除、预览和测试 API 不增加专用字段。

## 代码修改清单

### 后端领域逻辑

修改 `plugin/internal/domain/proxy.go`：

- 增加平台常量 `Generic Platform = "generic"`。
- `NormalizePlatform` 通过已有 Adapter 注册表接受 `generic`。
- 生成普通代理的账号 URL 时，直接返回保存后的 `BaseURL`，不改用户名或密码。
- 普通代理不需要稳定 hash；应在生成 URL 前判断平台是否需要账号身份，避免为 `generic` 计算 hash。

修改 `plugin/internal/domain/provider.go`：

- 在 `providers` 注册表中加入 `Generic: genericAdapter{}`。
- 新增 `genericAdapter`。它不额外校验用户名或密码，账号 URL 和测试 URL 都用 `Build(parts, parts.Username, parts.Password)` 原样构造。
- 为 Adapter 增加一个“是否需要测试 session”的内部能力标记。现有 Adapter 均返回需要，以保持原有行为；`generic` 返回不需要。

这样 `NewProxy`、账号状态识别、应用和按代理清除都会继续通过同一个 domain 入口工作，不需要在这些调用处判断 `generic`。

### 测试器

修改 `plugin/internal/tester/tester.go`：

- 仅当 Adapter 表明需要测试 session 时才调用 `RandomSessionID()`。
- 测试普通代理时直接使用其保存后的代理地址。
- 保持现有测试响应和 15 秒超时，不向页面返回完整 URL 或认证信息。

### 前端

修改下列文件，在平台选择和展示中加入普通代理：

| 文件 | 修改方式 |
| --- | --- |
| `plugin/web-ui/src/types.ts` | 在 `ProxyPlatform` 联合类型中加入 `"generic"`。 |
| `plugin/web-ui/src/components/ProxyForm.tsx` | 在新增、编辑代理的平台下拉框中加入“普通代理”。 |
| `plugin/web-ui/src/components/BulkProxyForm.tsx` | 在批量导入的平台下拉框中加入“普通代理”。 |
| `plugin/web-ui/src/components/ProxyList.tsx` | 为普通代理增加独立标记，例如 `G`，不能回退显示为 Decodo。 |
| `plugin/web-ui/src/styles.css` | 为 `.provider-mark.generic` 增加与现有平台一致的标记样式。 |
| `plugin/web-ui/src/i18n.ts` | 添加中英文名称“普通代理”，以及“不会保证粘滞出口”的简短说明。 |

### 不需要修改的调用逻辑

`plugin/internal/syncer`、`plugin/internal/accounts` 和 `plugin/internal/app` 不需要增加 `generic` 分支。它们已经通过 `domain.Rewrite` 生成预期 URL，所以普通代理的应用、状态识别、定向清除和回滚会自动沿用现有逻辑。

### 测试

修改或补充以下测试：

- `plugin/internal/domain/proxy_test.go`：验证 `generic` 可识别；无认证和带认证 URL 可保存；不同邮箱生成相同最终 URL；用户名、密码和 URL 编码保持不变。
- `plugin/internal/tester/tester_test.go`：验证普通代理测试地址等于保存后的代理地址，且不会请求或使用随机 session。
- `plugin/internal/syncer/syncer_test.go`：验证应用同一普通代理到多个账号时写入相同地址；按代理清除不会误清其他地址。
- `plugin/internal/accounts/accounts_test.go`：验证普通代理地址可被识别为已应用代理。
- 前端测试：验证 `generic` 可作为表单和批量导入的合法平台值，且代理列表显示普通代理标记。

实现后更新 `README.md` 的平台数量和平台表，并将本设计文档加入文档链接。

## 验收条件

1. 可以新增、编辑和批量导入 `generic` 代理。
2. 合法的无认证或带认证 HTTP、HTTPS、SOCKS5 地址均可保存。
3. 保存和应用不会改写用户名、密码、session、SID 或 TTL。
4. 不同账号使用同一普通代理时，最终代理地址相同，且等于保存后的代理地址。
5. 应用、重复应用、按代理清除和失败回滚遵守现有按账号范围规则。
6. 账号列表清楚展示“普通代理”，且不宣称粘滞性或账号隔离。
7. 测试不泄露完整代理地址或认证信息。
8. Go 测试、前端类型检查、前端测试和构建继续通过。
