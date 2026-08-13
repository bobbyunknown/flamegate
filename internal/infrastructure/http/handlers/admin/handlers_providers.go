package admin

// webProvider reports whether a provider is served by the web search/fetch
// connector (so it is routable even though its dialect is the generic openai).
func webProvider(id string) bool {
	switch id {
	case "tavily", "exa", "serper", "brave-search", "searxng", "firecrawl", "jina-reader":
		return true
	default:
		return false
	}
}

// adminProviderModels returns the model list for a specific provider. It
// includes static catalog models and, when a connected account exists, live
// models from the upstream (e.g. Kiro's ListAvailableModels).

// ---- API keys ---------------------------------------------------------------
