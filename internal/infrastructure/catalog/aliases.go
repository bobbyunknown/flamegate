package catalog

import "strings"

// ProviderAliases maps gateway or extension provider slugs to official models.dev provider IDs.
var ProviderAliases = map[string]string{
	"antigravity":   "google",
	"agy":           "google",
	"cline":         "anthropic",
	"cl":            "anthropic",
	"xiaomi-mimo":   "xiaomi",
	"mimo":          "xiaomi",
	"xm":            "xiaomi",
	"glm":           "zai",
	"glm-cn":        "zhipuai",
	"z-ai":          "zai",
	"z-ai-cn":       "zhipuai",
	"zhipu":         "zhipuai",
	"kimi":          "moonshotai",
	"kimi-cn":       "moonshotai-cn",
	"qwen":          "alibaba",
	"qwen-cn":       "alibaba-cn",
	"hunyuan":       "tencent",
	"doubao":        "volcengine",
	"deepseek-ai":   "deepseek",
	"cloudflare-ai": "cloudflare-workers-ai",
}

// ResolveProviderAlias returns the aliased models.dev provider ID if an alias exists;
// otherwise, it returns the normalized provider string.
func ResolveProviderAlias(provider string) string {
	norm := strings.ToLower(strings.TrimSpace(provider))
	if target, ok := ProviderAliases[norm]; ok {
		return target
	}
	return norm
}
