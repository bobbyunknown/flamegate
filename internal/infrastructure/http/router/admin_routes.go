package router

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/openapi"
)

func adminOp(id, method, path, summary, tag string, mw huma.Middlewares) huma.Operation {
	return huma.Operation{
		OperationID: id,
		Method:      method,
		Path:        path,
		Summary:     summary,
		Tags:        []string{tag},
		Security:    openapi.BearerSecurity,
		Middlewares: mw,
	}
}

func publicOp(id, method, path, summary, tag string, mw huma.Middlewares) huma.Operation {
	return huma.Operation{
		OperationID: id,
		Method:      method,
		Path:        path,
		Summary:     summary,
		Tags:        []string{tag},
		Security:    openapi.PublicSecurity,
		Middlewares: mw,
	}
}

func (s *Server) registerAdminAPI(api huma.API) {
	adminMw := []func(huma.Context, func(huma.Context)){
		openapi.LoopbackOnlyMiddleware(s.cfg),
		openapi.SessionAuthMiddleware(s.adminHandler.Auth()),
		openapi.InjectHTTPContext(),
	}
	authMw := []func(huma.Context, func(huma.Context)){
		openapi.LoopbackOnlyMiddleware(s.cfg),
		openapi.SessionAuthMiddleware(s.adminHandler.Auth()),
	}
	loginMw := []func(huma.Context, func(huma.Context)){
		openapi.LoopbackOnlyMiddleware(s.cfg),
		openapi.LoginRateLimitMiddleware(),
	}
	loopbackMw := []func(huma.Context, func(huma.Context)){
		openapi.LoopbackOnlyMiddleware(s.cfg),
	}

	loginOp := publicOp("post-login", "POST", "/auth/login", "Dashboard login (JSON, sets cookie)", "Auth", loginMw)
	loginOp.Description = "Authenticates with username/password and sets fg_session cookie."
	huma.Register(api, loginOp, s.adminHandler.HumaLogin)
	huma.Register(api, publicOp("get-auth-status", "GET", "/auth/status", "Auth status (onboarding, default password)", "Auth", loopbackMw), s.adminHandler.HumaAuthStatus)
	huma.Register(api, publicOp("post-logout", "POST", "/auth/logout", "Logout (clears session cookie)", "Auth", loopbackMw), s.adminHandler.HumaLogout)
	huma.Register(api, adminOp("change-password", "POST", "/auth/password", "Change password", "Auth", authMw), s.adminHandler.HumaChangePassword)
	huma.Register(api, adminOp("complete-onboarding", "POST", "/auth/onboarding/complete", "Complete onboarding", "Auth", authMw), s.adminHandler.HumaCompleteOnboarding)

	huma.Register(api, adminOp("list-accounts", "GET", "/accounts", "List provider accounts", "Accounts", adminMw), s.adminHandler.HumaListAccounts)
	huma.Register(api, adminOp("create-account", "POST", "/accounts", "Create provider account", "Accounts", adminMw), s.adminHandler.HumaCreateAccount)
	huma.Register(api, adminOp("bulk-create-accounts", "POST", "/accounts/bulk", "Bulk import provider accounts", "Accounts", adminMw), s.adminHandler.HumaBulkCreateAccounts)
	huma.Register(api, adminOp("validate-key", "POST", "/validate-key", "Validate provider credentials", "Accounts", adminMw), s.adminHandler.HumaValidateKey)
	huma.Register(api, adminOp("update-account", "PATCH", "/accounts/{id}", "Update account", "Accounts", adminMw), s.adminHandler.HumaUpdateAccount)
	huma.Register(api, adminOp("delete-account", "DELETE", "/accounts/{id}", "Delete account", "Accounts", adminMw), s.adminHandler.HumaDeleteAccount)
	huma.Register(api, adminOp("test-account", "POST", "/accounts/{id}/test", "Test account credentials", "Accounts", adminMw), s.adminHandler.HumaTestAccount)
	huma.Register(api, adminOp("account-quota", "GET", "/accounts/{id}/quota", "Get account quota", "Accounts", adminMw), s.adminHandler.HumaAccountQuota)

	huma.Register(api, adminOp("list-keys", "GET", "/keys", "List API keys", "Keys", adminMw), s.adminHandler.HumaListKeys)
	huma.Register(api, adminOp("create-key", "POST", "/keys", "Create API key", "Keys", adminMw), s.adminHandler.HumaCreateKey)
	huma.Register(api, adminOp("update-key", "PATCH", "/keys/{id}", "Update API key", "Keys", adminMw), s.adminHandler.HumaUpdateKey)
	huma.Register(api, adminOp("delete-key", "DELETE", "/keys/{id}", "Delete API key", "Keys", adminMw), s.adminHandler.HumaDeleteKey)
	huma.Register(api, adminOp("rotate-key", "POST", "/keys/{id}/rotate", "Rotate API key secret", "Keys", adminMw), s.adminHandler.HumaRotateKey)

	listChainsOp := adminOp("list-chains", "GET", "/chains", "List all chains", "Chains", adminMw)
	listChainsOp.Description = "Returns all routing chains for the admin tenant."
	huma.Register(api, listChainsOp, s.adminHandler.HumaListChains)
	createChainOp := adminOp("create-chain", "POST", "/chains", "Create a new chain", "Chains", adminMw)
	createChainOp.Description = "Creates a new routing chain with steps and optional fallback."
	huma.Register(api, createChainOp, s.adminHandler.HumaCreateChain)
	updateChainOp := adminOp("update-chain", "PATCH", "/chains/{id}", "Update a chain", "Chains", adminMw)
	updateChainOp.Description = "Updates an existing chain's configuration."
	huma.Register(api, updateChainOp, s.adminHandler.HumaUpdateChain)
	deleteChainOp := adminOp("delete-chain", "DELETE", "/chains/{id}", "Delete a chain", "Chains", adminMw)
	deleteChainOp.Description = "Deletes a chain by ID."
	huma.Register(api, deleteChainOp, s.adminHandler.HumaDeleteChain)

	listPlansOp := adminOp("list-plans", "GET", "/plans", "List all plans", "Plans", adminMw)
	listPlansOp.Description = "Returns all billing plans for the admin tenant."
	huma.Register(api, listPlansOp, s.adminHandler.HumaListPlans)
	createPlanOp := adminOp("create-plan", "POST", "/plans", "Create a new plan", "Plans", adminMw)
	createPlanOp.Description = "Creates a new billing plan with limits and allowed models."
	huma.Register(api, createPlanOp, s.adminHandler.HumaCreatePlan)
	updatePlanOp := adminOp("update-plan", "PATCH", "/plans/{id}", "Update a plan", "Plans", adminMw)
	updatePlanOp.Description = "Updates an existing plan's configuration."
	huma.Register(api, updatePlanOp, s.adminHandler.HumaUpdatePlan)
	deletePlanOp := adminOp("delete-plan", "DELETE", "/plans/{id}", "Delete a plan", "Plans", adminMw)
	deletePlanOp.Description = "Deletes a plan by ID. Fails if keys are still assigned."
	huma.Register(api, deletePlanOp, s.adminHandler.HumaDeletePlan)
	listPlanKeysOp := adminOp("list-plan-keys", "GET", "/plans/{id}/keys", "List keys assigned to a plan", "Plans", adminMw)
	listPlanKeysOp.Description = "Returns all API keys assigned to the specified plan."
	huma.Register(api, listPlanKeysOp, s.adminHandler.HumaListPlanKeys)

	huma.Register(api, adminOp("list-budgets", "GET", "/budgets", "List all budgets", "Budgets", adminMw), s.adminHandler.HumaListBudgets)
	huma.Register(api, adminOp("budget-status", "GET", "/budgets/status", "Get budget status with spend data", "Budgets", adminMw), s.adminHandler.HumaBudgetStatus)
	huma.Register(api, adminOp("create-budget", "POST", "/budgets", "Create a new budget", "Budgets", adminMw), s.adminHandler.HumaCreateBudget)
	huma.Register(api, adminOp("update-budget", "PATCH", "/budgets/{id}", "Update a budget", "Budgets", adminMw), s.adminHandler.HumaUpdateBudget)
	huma.Register(api, adminOp("delete-budget", "DELETE", "/budgets/{id}", "Delete a budget", "Budgets", adminMw), s.adminHandler.HumaDeleteBudget)

	huma.Register(api, adminOp("usage-summary", "GET", "/usage", "Get usage summary", "Usage", adminMw), s.adminHandler.HumaUsageSummary)
	huma.Register(api, adminOp("usage-insights", "GET", "/usage/insights", "Get usage insights with analytics", "Usage", adminMw), s.adminHandler.HumaUsageInsights)
	huma.Register(api, adminOp("model-usage", "GET", "/usage/models", "Get per-model usage breakdown", "Usage", adminMw), s.adminHandler.HumaModelUsage)
	huma.Register(api, adminOp("quota-usage", "GET", "/quota", "Get account quota usage", "Usage", adminMw), s.adminHandler.HumaQuotaUsage)

	huma.Register(api, adminOp("listProviders", "GET", "/providers", "List all providers", "Providers", adminMw), s.adminHandler.HumaListProviders)
	huma.Register(api, adminOp("getProviderModels", "GET", "/providers/{id}/models", "Get models for a provider (optional ?refresh=true to import)", "Providers", adminMw), s.adminHandler.HumaProviderModels)
	clearProviderModelsOp := adminOp("clearProviderDiscoveredModels", "DELETE", "/providers/{id}/models", "Clear or delete discovered/imported models for a provider (keeps static + custom)", "Providers", adminMw)
	clearProviderModelsOp.DefaultStatus = 200
	huma.Register(api, clearProviderModelsOp, s.adminHandler.HumaClearProviderDiscoveredModels)
	bulkDeleteProviderModelsOp := adminOp("bulkDeleteProviderModels", "POST", "/providers/{id}/models/bulk-delete", "Bulk delete selected models for a provider", "Providers", adminMw)
	bulkDeleteProviderModelsOp.DefaultStatus = 200
	huma.Register(api, bulkDeleteProviderModelsOp, s.adminHandler.HumaBulkDeleteProviderModels)
	huma.Register(api, adminOp("getProviderRouting", "GET", "/providers/{id}/routing", "Get provider routing settings", "Providers", adminMw), s.adminHandler.HumaGetProviderRouting)
	updateProviderRoutingOp := adminOp("updateProviderRouting", "POST", "/providers/{id}/routing", "Update provider routing settings", "Providers", adminMw)
	updateProviderRoutingOp.DefaultStatus = 200
	huma.Register(api, updateProviderRoutingOp, s.adminHandler.HumaUpdateProviderRouting)
	patchProviderRoutingOp := adminOp("patchProviderRouting", "PATCH", "/providers/{id}/routing", "Update provider routing settings", "Providers", adminMw)
	patchProviderRoutingOp.DefaultStatus = 200
	huma.Register(api, patchProviderRoutingOp, s.adminHandler.HumaUpdateProviderRouting)

	huma.Register(api, adminOp("getEndpointSettings", "GET", "/settings/endpoint", "Get endpoint settings", "Settings", authMw), s.adminHandler.HumaGetEndpointSettings)
	updateEndpointSettingsOp := adminOp("updateEndpointSettings", "POST", "/settings/endpoint", "Update endpoint settings", "Settings", authMw)
	updateEndpointSettingsOp.DefaultStatus = 200
	huma.Register(api, updateEndpointSettingsOp, s.adminHandler.HumaUpdateEndpointSettings)
	testHeadroomOp := adminOp("testHeadroom", "POST", "/settings/headroom-test", "Test Headroom proxy connection", "Settings", authMw)
	testHeadroomOp.DefaultStatus = 200
	huma.Register(api, testHeadroomOp, s.adminHandler.HumaTestHeadroom)
	huma.Register(api, adminOp("getAccessSettings", "GET", "/settings/access", "Get access settings", "Settings", authMw), s.adminHandler.HumaGetAccessSettings)
	updateAccessSettingsOp := adminOp("updateAccessSettings", "POST", "/settings/access", "Update access settings", "Settings", authMw)
	updateAccessSettingsOp.DefaultStatus = 200
	huma.Register(api, updateAccessSettingsOp, s.adminHandler.HumaUpdateAccessSettings)

	huma.Register(api, adminOp("listAliases", "GET", "/models/alias", "List all model aliases", "Models", authMw), s.adminHandler.HumaListAliases)
	setAliasOp := adminOp("setAlias", "PUT", "/models/alias", "Set a model alias", "Models", authMw)
	setAliasOp.DefaultStatus = 200
	huma.Register(api, setAliasOp, s.adminHandler.HumaSetAlias)
	deleteAliasOp := adminOp("deleteAlias", "DELETE", "/models/alias", "Delete a model alias", "Models", authMw)
	deleteAliasOp.DefaultStatus = 200
	huma.Register(api, deleteAliasOp, s.adminHandler.HumaDeleteAlias)
	huma.Register(api, adminOp("listDisabledModels", "GET", "/models/disabled", "List disabled models for a provider", "Models", authMw), s.adminHandler.HumaListDisabledModels)
	disableModelsOp := adminOp("disableModels", "POST", "/models/disabled", "Disable models for a provider", "Models", authMw)
	disableModelsOp.DefaultStatus = 200
	huma.Register(api, disableModelsOp, s.adminHandler.HumaDisableModels)
	enableModelsOp := adminOp("enableModels", "DELETE", "/models/disabled", "Enable models for a provider", "Models", authMw)
	enableModelsOp.DefaultStatus = 200
	huma.Register(api, enableModelsOp, s.adminHandler.HumaEnableModels)

	testModelOp := adminOp("testModel", "POST", "/models/test", "Test model connectivity and latency", "Models", authMw)
	testModelOp.DefaultStatus = 200
	huma.Register(api, testModelOp, s.adminHandler.HumaTestModel)

	testChatOp := adminOp("testChat", "POST", "/models/chat", "Interactive chat test for admin playground", "Models", authMw)
	testChatOp.DefaultStatus = 200
	huma.Register(api, testChatOp, s.adminHandler.HumaTestChat)

	huma.Register(api, adminOp("list-guardrails", "GET", "/guardrails", "List guardrails", "Guardrails", authMw), s.adminHandler.HumaListGuardrails)
	huma.Register(api, adminOp("create-guardrail", "POST", "/guardrails", "Create guardrail", "Guardrails", authMw), s.adminHandler.HumaCreateGuardrail)
	huma.Register(api, adminOp("get-guardrail", "GET", "/guardrails/{id}", "Get guardrail", "Guardrails", authMw), s.adminHandler.HumaGetGuardrail)
	huma.Register(api, adminOp("update-guardrail", "PATCH", "/guardrails/{id}", "Update guardrail", "Guardrails", authMw), s.adminHandler.HumaUpdateGuardrail)
	huma.Register(api, adminOp("delete-guardrail", "DELETE", "/guardrails/{id}", "Delete guardrail", "Guardrails", authMw), s.adminHandler.HumaDeleteGuardrail)
	huma.Register(api, adminOp("effective-guardrail", "GET", "/guardrails/effective", "Get effective guardrail", "Guardrails", authMw), s.adminHandler.HumaEffectiveGuardrail)
	huma.Register(api, adminOp("list-guardrail-entities", "GET", "/guardrails/entities", "List guardrail entities", "Guardrails", authMw), s.adminHandler.HumaListGuardrailEntities)
	huma.Register(api, adminOp("list-guardrail-logs", "GET", "/guardrails/logs", "List guardrail logs", "Guardrails", authMw), s.adminHandler.HumaListGuardrailLogs)
	huma.Register(api, adminOp("test-guardrail", "POST", "/guardrails/test", "Test guardrail", "Guardrails", authMw), s.adminHandler.HumaTestGuardrail)

	huma.Register(api, adminOp("system-status", "GET", "/system", "Get system status", "System", authMw), s.adminHandler.HumaSystemStatus)
	huma.Register(api, adminOp("system-history", "GET", "/system/history", "Get system history", "System", authMw), s.adminHandler.HumaSystemHistory)
	huma.Register(api, adminOp("system-resources", "GET", "/system/resources", "Get system resource history", "System", authMw), s.adminHandler.HumaSystemResources)
	huma.Register(api, adminOp("update-check", "GET", "/update/check", "Check for updates", "System", authMw), s.adminHandler.HumaUpdateCheck)

	huma.Register(api, adminOp("tunnel-status", "GET", "/tunnel/status", "Get tunnel status", "Tunnel", authMw), s.adminHandler.HumaTunnelStatus)
	huma.Register(api, adminOp("tunnel-enable", "POST", "/tunnel/enable", "Enable tunnel", "Tunnel", authMw), s.adminHandler.HumaTunnelEnable)
	huma.Register(api, adminOp("tunnel-disable", "POST", "/tunnel/disable", "Disable tunnel", "Tunnel", authMw), s.adminHandler.HumaTunnelDisable)
	huma.Register(api, adminOp("tailscale-check", "GET", "/tunnel/tailscale-check", "Check Tailscale status", "Tunnel", authMw), s.adminHandler.HumaTailscaleCheck)
	huma.Register(api, adminOp("tailscale-enable", "POST", "/tunnel/tailscale-enable", "Enable Tailscale", "Tunnel", authMw), s.adminHandler.HumaTailscaleEnable)
	huma.Register(api, adminOp("tailscale-disable", "POST", "/tunnel/tailscale-disable", "Disable Tailscale", "Tunnel", authMw), s.adminHandler.HumaTailscaleDisable)

	huma.Register(api, adminOp("list-proxy-pools", "GET", "/proxy-pools", "List proxy pools", "Proxy Pools", authMw), s.adminHandler.HumaListProxyPools)
	huma.Register(api, adminOp("create-proxy-pool", "POST", "/proxy-pools", "Create proxy pool", "Proxy Pools", authMw), s.adminHandler.HumaCreateProxyPool)
	huma.Register(api, adminOp("update-proxy-pool", "PATCH", "/proxy-pools/{id}", "Update proxy pool", "Proxy Pools", authMw), s.adminHandler.HumaUpdateProxyPool)
	huma.Register(api, adminOp("delete-proxy-pool", "DELETE", "/proxy-pools/{id}", "Delete proxy pool", "Proxy Pools", authMw), s.adminHandler.HumaDeleteProxyPool)
	huma.Register(api, adminOp("test-proxy-pool", "POST", "/proxy-pools/{id}/test", "Test proxy pool", "Proxy Pools", authMw), s.adminHandler.HumaTestProxyPool)

	huma.Register(api, adminOp("list-skills", "GET", "/skills", "List skills", "Skills", authMw), s.adminHandler.HumaListSkills)
	huma.Register(api, adminOp("create-skill", "POST", "/skills", "Create skill", "Skills", authMw), s.adminHandler.HumaCreateSkill)
	huma.Register(api, adminOp("update-skill", "POST", "/skills/{id}", "Update skill", "Skills", authMw), s.adminHandler.HumaUpdateSkill)
	huma.Register(api, adminOp("delete-skill", "DELETE", "/skills/{id}", "Delete skill", "Skills", authMw), s.adminHandler.HumaDeleteSkill)

	huma.Register(api, adminOp("list-account-health", "GET", "/health/accounts", "List account health", "Health", authMw), s.adminHandler.HumaListAccountHealth)
	huma.Register(api, adminOp("run-health-check", "POST", "/health/check-now", "Run health check", "Health", authMw), s.adminHandler.HumaRunHealthCheck)
}
