# StickyProxy / 粘滞代理

[![Release](https://img.shields.io/github/v/release/KonstantinosZornes/CPAStickyProxy?display_name=tag)](https://github.com/KonstantinosZornes/CPAStickyProxy/releases)
[![License](https://img.shields.io/github/license/KonstantinosZornes/CPAStickyProxy)](LICENSE)

**StickyProxy 是 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 的原生插件**，可以为不同账号分别配置上游代理。插件会根据账号邮箱生成稳定的会话标识，让同一账号重复使用同一代理时，尽量保持出口 IP 不变。

> 插件直接在 CLIProxyAPI 服务端保存账号代理配置，不需要额外启动 Python、Node.js 或本地代理服务。完整代理地址不会经过浏览器页面。

## 项目截图

### 主界面

<p align="center">
  <img width="3000" height="1662" alt="StickyProxy 主界面" src="https://github.com/user-attachments/assets/ddc41979-699d-425f-9566-9ae86bd87d04" />
</p>

### 应用代理界面

<p align="center">
  <img width="1200" alt="StickyProxy 应用代理界面" src="https://github.com/user-attachments/assets/8c55eb6b-abf4-47f5-b893-5061fef50573" />
</p>

## 它能做什么？

- **管理多个代理**：保存代理名称、平台和基础代理地址，随时新增、编辑或删除。
- **为账号保持固定会话**：粘滞平台会让同一账号使用相同的会话标识。
- **只修改选中的账号**：同步前可筛选并勾选账号；没选中的账号不会被改动。
- **支持四家粘滞平台和普通代理**：Decodo、DataImpulse、Resin、1024Proxy 和普通代理。
- **测试代理线路**：查看本次测试的出口 IP、国家、地区和城市；粘滞平台不会占用账号正在使用的会话。
- **按需清理代理**：可清除指定账号的代理；取消代理时也只会清理真正使用它的账号。

### 工作方式

```text
选择的代理
        ↓
粘滞平台：按账号邮箱生成固定会话标识
普通代理：直接使用保存后的代理地址
        ↓
按平台规则生成账号代理地址
        ↓
保存到该账号的代理设置
        ↓
CPA 使用该代理访问外部服务
```

账号单独设置的代理优先于 CPA 的全局代理。因此，没有单独配置代理的账号，仍会使用全局代理或直接连接。

## 支持的平台与协议

| 平台 | 账号代理地址生成规则 |
| --- | --- |
| Decodo | 在用户名中加入 `-session-<hash>-sessionduration-30` |
| DataImpulse | 在用户名中加入 `;sessid.<hash>` |
| Resin | 使用 `Platform.sp_<hash>` 作为认证用户名 |
| 1024Proxy | 替换或加入用户名中的 `-sid-<hash>`，原有 `-t-...` TTL 和定位参数保持不变 |
| 普通代理 | 直接使用保存后的代理地址，不注入账号 session，也不保证出口 IP 粘滞性 |

可使用 `http://`、`https://` 和 `socks5://` 代理地址；输入时也兼容 `socks://` 与 `socks5h://`。其他协议无法保存。

## 安装

### 通过 CPA 插件商店安装（推荐）

StickyProxy 目前还没有进入官方插件仓库，但可以添加本项目的插件商店来源后安装：

1. 打开 CLIProxyAPI（CPA）的 **系统设置**。
2. 在“插件商店来源”中添加下面的地址：

   ```text
   https://raw.githubusercontent.com/KonstantinosZornes/CPAStickyProxy/main/registry.json
   ```

3. 刷新插件列表，搜索 **CPAStickyProxy**（插件 ID：`stickyproxy`）。
4. 点击安装，并在插件管理中启用 StickyProxy。CPA 会自动下载适合当前系统和 CPU 架构的版本。
5. 从 CPA 主页面的插件菜单打开 **StickyProxy**。如果你已登录 CPA 管理中心，插件会自动使用已有登录状态。

### 从源码构建（Linux，开发用途）

这一节只适合本地开发和验证；日常安装请使用上面的 CPA 插件商店。构建需要 **Node.js 24+、pnpm、Go、CGO 和 C 编译器**，建议使用与目标 CLIProxyAPI 相同的 Go 版本。

```bash
git clone https://github.com/KonstantinosZornes/CPAStickyProxy.git
cd CPAStickyProxy/plugin/web-ui
pnpm install --ignore-scripts

cd ../..
./plugin/build-linux.sh
```

如果目标项目要求指定 Go 版本，可以这样构建：

```bash
GOTOOLCHAIN=go1.26.0 ./plugin/build-linux.sh
```

## 快速使用

1. 打开 StickyProxy，添加一条代理：填写名称、平台和基础代理地址。
2. 选择要使用的代理，打开账号选择页面。
3. 按服务商、账号类型、状态、优先级或备注筛选账号，勾选这次需要应用代理的账号。
4. 确认同步。插件会按平台规则为每个已选账号生成代理地址，并保存到它的账号设置中。
5. 想确认线路是否可用时，点击代理的**测试**按钮，查看出口 IP 和所在地。

> 同步会覆盖已选账号原来的代理设置，且不会备份旧值；没有选中的账号不会受影响。如果部分账号同步失败，已成功处理的账号会保留修改，失败账号会在结果中列出；修复后可只重试失败账号。

## 数据与安全说明

StickyProxy 会保存你添加的代理配置与账号代理映射；映射仅包含 CPA 的 `auth_index` 和代理 `proxy_id`。它不会保存账号邮箱、会话标识、最终代理地址、用量或 IP 历史。例如：

```json
{
  "proxies": [
    {
      "name": "US-Decodo",
      "platform": "decodo",
      "base_url": "http://user:password@gate.decodo.com:7000"
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

> **升级注意：** 当前代理状态格式不读取旧版本的状态数据。升级前请备份状态文件；升级后需重新添加代理并重新应用给账号。

> 代理地址中可能包含代理密码或 Resin Token。按照当前设计，这些内容会以**明文**保存在 CLIProxyAPI 工作目录下的 `plugins/stickyproxy-state` 文件中。文件权限为 `0600`，创建的 `plugins` 目录权限为 `0700`。请妥善保护 CLIProxyAPI 的工作目录及其备份文件。

## 开发与验证

前端源码在 `plugin/web-ui/src/`，构建后会写入 `plugin/web/dist/index.html`；请不要手动修改生成文件。

```bash
# 前端检查、测试和构建
cd plugin/web-ui
pnpm run check
pnpm test
pnpm run build

# 原生插件测试
cd ../
go test ./...
```

## 发布（维护者）

发布新版本时，GitHub Actions 会生成适用于 Linux、macOS 和 Windows 的插件安装包，并提供 `checksums.txt` 校验文件。当前 StickyProxy 通过项目自带的 [`registry.json`](https://raw.githubusercontent.com/KonstantinosZornes/CPAStickyProxy/main/registry.json) 提供给 CPA 插件商店。正式进入官方 [CLIProxyAPI-Plugins-Store](https://github.com/router-for-me/CLIProxyAPI-Plugins-Store) 后，用户将不再需要手动添加这个来源。

## 文档

- [完整设计](docs/001-cliproxy-sticky-plugin-design.md)
- [账号选择与大规模同步设计](docs/002-cliproxy-account-selection-scale-design.md)
- [1024Proxy 平台设计](docs/003-add-1024proxy-platform.md)
- [普通代理平台设计](docs/004-add-generic-proxy-platform.md)
- [账号代理映射设计](docs/005-persisted-account-proxy-mapping.md)
- [领域词汇](CONTEXT.md)

## 友链

- [Linux.do](https://linux.do)

## 许可证

[MIT](LICENSE)
