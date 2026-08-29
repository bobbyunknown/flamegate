// Re-export all types
export * from "./types";

// Re-export request infrastructure
export { APIError, BASE_URL } from "./client";

// Re-export extension store types for consumers
export type { StoreExtension } from "./extensions";

// Re-export standalone functions (for direct use)
export { connectUsageStream, connectGuardrailLogStream } from "./sse";
export { fetchPortalBranding, fetchKeyUsage, fetchKeyUsageById } from "./portal";

// Import all domain functions
import { authStatus, login, logout, changePassword, completeOnboarding } from "./auth";
import {
  providers, providerModels, syncProviderModels, deleteProviderModel, clearProviderModels, bulkDeleteProviderModels,
  providerRouting, updateProviderRouting,
  listCustomProviders, createCustomProvider, updateCustomProvider, deleteCustomProvider,
  listCustomModels, createCustomModel, updateCustomModel, deleteCustomModel,
} from "./providers";
import {
  listExtensions, getExtension, installExtension, uninstallExtension,
  enableExtension, disableExtension, syncExtensionModels, setExtensionAutoSyncModels,
  listStoreExtensions, getStoreExtension, installRemoteExtension, updateRemoteExtension,
} from "./extensions";
import { listPlans, createPlan, updatePlan, deletePlan, listPlanKeys } from "./plans";
import { listKeys, createKey, updateKey, deleteKey, deleteKeys } from "./keys";
import {
  listAccounts, createAccount, bulkCreateAccounts, updateAccount,
  deleteAccount, testAccount, validateKey, accountQuota,
} from "./accounts";
import { listChains, createChain, updateChain, deleteChain } from "./chains";
import { listBudgets, budgetStatus, createBudget, updateBudget, deleteBudget } from "./budgets";
import { usage, usageInsights, modelUsage } from "./usage";
import { quota, quotaByProvider } from "./quota";
import { consoleLog } from "./console";
import { cliTools, cliToolConfigure, cliToolRemove } from "./cli-tools";
import { listProxyPools, createProxyPool, updateProxyPool, deleteProxyPool } from "./proxy-pools";
import { listSkills, createSkill, updateSkill, deleteSkill } from "./skills";
import {
  endpointSettings, updateEndpointSettings, testHeadroom,
  accessSettings, updateAccessSettings, branding, updateBranding,
} from "./settings";
import {
  tunnelStatus, tunnelEnable, tunnelDisable,
  tailscaleCheck, tailscaleEnable, tailscaleDisable,
} from "./tunnel";
import { listDisabledModels, disableModels, enableModels } from "./models";
import {
  updateCheck, exportDatabase, importDatabase,
  sqliteStatus, backupSQLite, restoreSQLite,
} from "./database";
import { testProxy, testProxyPool } from "./proxy-test";
import {
  kiroDeviceStart, kiroDevicePoll, kiroAPIKey, kiroImport,
  qoderDeviceStart, qoderDevicePoll,
  kilocodeDeviceStart, kilocodeDevicePoll,
  codebuddyAuthStart, codebuddyAuthPoll,
  cursorImport, commandcodeImport, oauthStart, oauthExchange,
} from "./connectors";
import { systemMonitor, systemHistory } from "./system";
import {
  listGuardrails, getGuardrail, createGuardrail, updateGuardrail, deleteGuardrail,
  effectiveGuardrail, listGuardrailEntities, listGuardrailLogs, testGuardrail,
  listGuardrailTemplates, exportGuardrails, importGuardrails,
  getGuardrailTenantFlags, putGuardrailTenantFlags,
} from "./guardrails";

// Construct and export the unified `api` object
export const api = {
  // Auth
  authStatus, login, logout, changePassword, completeOnboarding,

  // Providers
  providers, providerModels, syncProviderModels, deleteProviderModel, clearProviderModels, bulkDeleteProviderModels,
  providerRouting, updateProviderRouting,
  listCustomProviders, createCustomProvider, updateCustomProvider, deleteCustomProvider,
  listCustomModels, createCustomModel, updateCustomModel, deleteCustomModel,

  // Extensions (WASM providers)
  listExtensions, getExtension, installExtension, uninstallExtension,
  enableExtension, disableExtension, syncExtensionModels, setExtensionAutoSyncModels,

  // Extensions Store (remote install)
  listStoreExtensions, getStoreExtension, installRemoteExtension, updateRemoteExtension,
  // Per-extension OAuth (host gateway)
  oauthStart, oauthExchange,

  // Plans
  listPlans, createPlan, updatePlan, deletePlan, listPlanKeys,

  // Keys
  listKeys, createKey, updateKey, deleteKey, deleteKeys,

  // Accounts
  listAccounts, createAccount, bulkCreateAccounts, updateAccount,
  deleteAccount, testAccount, validateKey, accountQuota,

  // Chains
  listChains, createChain, updateChain, deleteChain,

  // Budgets
  listBudgets, budgetStatus, createBudget, updateBudget, deleteBudget,

  // Usage
  usage, usageInsights, modelUsage,

  // Quota
  quota, quotaByProvider,

  // Console
  consoleLog,

  // CLI Tools
  cliTools, cliToolConfigure, cliToolRemove,

  // Proxy Pools
  listProxyPools, createProxyPool, updateProxyPool, deleteProxyPool,

  // Skills
  listSkills, createSkill, updateSkill, deleteSkill,

  // Settings
  endpointSettings, updateEndpointSettings, testHeadroom,
  accessSettings, updateAccessSettings, branding, updateBranding,

  // Tunnel
  tunnelStatus, tunnelEnable, tunnelDisable,
  tailscaleCheck, tailscaleEnable, tailscaleDisable,

  // Models
  listDisabledModels, disableModels, enableModels,

  // Database / Updates
  updateCheck, exportDatabase, importDatabase,
  sqliteStatus, backupSQLite, restoreSQLite,

  // Proxy Test
  testProxy, testProxyPool,

  // Connectors
  kiroDeviceStart, kiroDevicePoll, kiroAPIKey, kiroImport,
  qoderDeviceStart, qoderDevicePoll,
  kilocodeDeviceStart, kilocodeDevicePoll,
  codebuddyAuthStart, codebuddyAuthPoll,
  cursorImport, commandcodeImport,

  // System
  systemMonitor, systemHistory,

  // Guardrails
  listGuardrails, getGuardrail, createGuardrail, updateGuardrail, deleteGuardrail,
  effectiveGuardrail, listGuardrailEntities, listGuardrailLogs, testGuardrail,
  listGuardrailTemplates, exportGuardrails, importGuardrails,
  getGuardrailTenantFlags, putGuardrailTenantFlags,
};
