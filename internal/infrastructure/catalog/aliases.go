package catalog

import "strings"

// ProviderAliases maps gateway or extension provider slugs to official models.dev provider IDs.
var ProviderAliases = map[string]string{
	"antigravity":   "google",
	"cline":         "anthropic",
	"xiaomi-mimo":   "xiaomi",
	"glm":           "zai",
	"glm-cn":        "zhipuai",
	"zhipu":         "zhipuai",
	"kimi":          "moonshotai",
	"kimi-cn":       "moonshotai-cn",
	"qwen":          "alibaba",
	"qwen-cn":       "alibaba-cn",
	"hunyuan":       "tencent",
	"doubao":        "volcengine",
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
