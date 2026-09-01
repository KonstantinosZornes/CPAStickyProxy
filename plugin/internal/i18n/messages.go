// Package i18n contains the small Chinese/English catalog used by the native
// management plugin. Keeping error codes separate from text lets the page and
// API remain bilingual without leaking implementation errors to users.
package i18n

import (
	"errors"
	"fmt"
	"strings"
)

type Locale string

const (
	ZH Locale = "zh"
	EN Locale = "en"
)

var catalog = map[Locale]map[string]string{
	ZH: {
		"invalid_email":               "请输入有效邮箱",
		"invalid_platform":            "不支持的平台：%s",
		"proxy_empty":                 "代理地址不能为空",
		"invalid_proxy":               "代理地址无效：%s",
		"unsupported_scheme":          "仅支持 HTTP、HTTPS、SOCKS5 代理",
		"missing_username":            "%s 代理必须配置用户名",
		"missing_proxy_password":      "%s 代理必须配置密码",
		"missing_resin_token":         "Resin 代理必须配置 Proxy Token（代理密码）",
		"invalid_1024_sid":            "1024Proxy 用户名中的 session ID 格式无效",
		"proxy_not_found":             "代理不存在",
		"duplicate_proxy":             "相同平台和基础代理地址的代理已存在",
		"state_read_failed":           "读取插件状态失败",
		"state_write_failed":          "保存插件状态失败",
		"invalid_request":             "请求体必须是 JSON 对象",
		"invalid_json":                "JSON 请求体无效：%s",
		"not_found":                   "接口不存在",
		"sync_failed":                 "应用或清除账号代理失败",
		"sync_unavailable":            "当前插件宿主不支持账号同步回调",
		"preview_failed":              "无法预览所选代理规则",
		"proxy_static":                "该 Decodo 代理使用端口型粘滞，未注入 session 参数",
		"test_failed":                 "代理出口测试失败",
		"selection_required":          "请至少选择一个账号",
		"selection_too_large":         "一次最多选择 %d 个账号，请缩小筛选条件",
		"account_not_found":           "所选账号已不存在，请刷新账号列表",
		"account_not_selectable":      "所选账号不可应用或清除代理",
		"duplicate_account_selection": "账号选择中存在重复项",
		"accounts_load_failed":        "读取账号代理配置失败",
		"invalid_proxy_name":          "代理名称仅允许大小写字母、数字、短横线和下划线",
	},
	EN: {
		"invalid_email":               "Enter a valid email address",
		"invalid_platform":            "Unsupported platform: %s",
		"proxy_empty":                 "Proxy address cannot be empty",
		"invalid_proxy":               "Invalid proxy address: %s",
		"unsupported_scheme":          "Only HTTP, HTTPS, and SOCKS5 proxies are supported",
		"missing_username":            "%s proxy requires a username",
		"missing_proxy_password":      "%s requires a proxy password",
		"missing_resin_token":         "A Resin proxy requires a Proxy Token as its proxy password",
		"invalid_1024_sid":            "The 1024Proxy username has an invalid session ID",
		"proxy_not_found":             "Proxy not found",
		"duplicate_proxy":             "A proxy with the same platform and base URL already exists",
		"state_read_failed":           "Could not read plugin state",
		"state_write_failed":          "Could not save plugin state",
		"invalid_request":             "Request body must be a JSON object",
		"invalid_json":                "Invalid JSON request body: %s",
		"not_found":                   "Route not found",
		"sync_failed":                 "Could not apply or clear account proxies",
		"sync_unavailable":            "The current plugin host does not support account synchronization callbacks",
		"preview_failed":              "Could not preview the selected proxy rule",
		"proxy_static":                "This Decodo proxy uses port-based stickiness; no session parameter was injected",
		"test_failed":                 "Proxy egress test failed",
		"selection_required":          "Select at least one account",
		"selection_too_large":         "Select at most %d accounts at once; narrow the filters",
		"account_not_found":           "A selected account no longer exists; refresh the account list",
		"account_not_selectable":      "A selected account cannot receive this proxy operation",
		"duplicate_account_selection": "The account selection contains duplicates",
		"accounts_load_failed":        "Could not load account proxy configuration",
		"invalid_proxy_name":          "Proxy name may contain only letters, numbers, hyphens, and underscores",
	},
}

// Error propagates an implementation-independent code to the HTTP seam.
type Error struct {
	Code string
	Args []any
}

func (e Error) Error() string { return Message(e, ZH) }

func New(code string, args ...any) error { return Error{Code: code, Args: args} }

func ParseLocale(value string) Locale {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "en") {
		return EN
	}
	return ZH
}

func FromHeaders(headers map[string][]string) Locale {
	for key, values := range headers {
		if !strings.EqualFold(key, "Accept-Language") || len(values) == 0 {
			continue
		}
		return ParseLocale(strings.Split(values[0], ",")[0])
	}
	return ZH
}

func Code(err error) string {
	var typed Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return "internal_error"
}

func Message(err error, locale Locale) string {
	var typed Error
	if errors.As(err, &typed) {
		return Text(locale, typed.Code, typed.Args...)
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

func Text(locale Locale, code string, args ...any) string {
	messages := catalog[locale]
	if messages == nil {
		messages = catalog[ZH]
	}
	message := messages[code]
	if message == "" {
		message = catalog[ZH][code]
	}
	if message == "" {
		message = code
	}
	if len(args) > 0 {
		return fmt.Sprintf(message, args...)
	}
	return message
}
