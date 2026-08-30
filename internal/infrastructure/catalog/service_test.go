package catalog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/catalog"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/meter"
)

const sampleCatalogJSON = `{
	"google": {
		"id": "google",
		"name": "Google",
		"models": {
			"gemini-2.5-flash": {
				"id": "gemini-2.5-flash",
				"name": "Gemini 2.5 Flash",
				"tool_call": true,
				"reasoning": true,
				"modalities": {
					"input": ["text", "image", "audio", "video", "pdf"],
					"output": ["text"]
				},
				"cost": {
					"input": 0.3,
					"output": 2.5,
					"cache_read": 0.03,
					"cache_write": 0.0,
					"reasoning": 2.5
				},
				"limit": {
					"context": 1048576,
					"output": 65536
				}
			},
			"gemini-2.5-pro": {
				"id": "gemini-2.5-pro",
				"name": "Gemini 2.5 Pro",
				"tool_call": true,
				"cost": {
					"input": 1.25,
					"output": 5.0
				}
			}
		}
	},
	"openai": {
		"id": "openai",
		"name": "OpenAI",
		"models": {
			"gpt-4o": {
				"id": "gpt-4o",
				"name": "GPT-4o",
				"tool_call": true,
				"cost": {
					"input": 2.5,
					"output": 10.0,
					"cache_read": 1.25,
					"cache_write": 3.75
				},
				"limit": {
					"context": 128000,
					"output": 16384
				}
			}
		}
	},
	"anthropic": {
		"id": "anthropic",
		"name": "Anthropic",
		"models": {
			"claude-3-7-sonnet": {
				"id": "claude-3-7-sonnet",
				"name": "Claude 3.7 Sonnet",
				"tool_call": true,
				"reasoning": true,
				"cost": {
					"input": 3.0,
					"output": 15.0,
					"cache_read": 0.3,
					"cache_write": 3.75
				},
				"limit": {
					"context": 200000,
					"output": 64000
				}
			}
		}
	},
	"xiaomi": {
		"id": "xiaomi",
		"name": "Xiaomi",
		"models": {
			"mimo-v1": {
				"id": "mimo-v1",
				"name": "MiMo v1",
				"cost": {
					"input": 0.5,
					"output": 1.5
				}
			}
		}
	},
	"zai": {
		"id": "zai",
		"name": "Zhipu AI International",
		"models": {
			"glm-4-plus": {
				"id": "glm-4-plus",
				"name": "GLM 4 Plus",
				"cost": {
					"input": 1.0,
					"output": 2.0
				}
			}
		}
	},
	"zhipuai": {
		"id": "zhipuai",
		"name": "Zhipu AI China",
		"models": {
			"glm-4-air": {
				"id": "glm-4-air",
				"name": "GLM 4 Air",
				"cost": {
					"input": 0.1,
					"output": 0.2
				}
			}
		}
	},
	"moonshotai": {
		"id": "moonshotai",
		"name": "Moonshot AI",
		"models": {
			"moonshot-v1-8k": {
				"id": "moonshot-v1-8k",
				"name": "Moonshot v1 8k",
				"cost": {
					"input": 0.2,
					"output": 0.5
				}
			}
		}
	},
	"moonshotai-cn": {
		"id": "moonshotai-cn",
		"name": "Moonshot AI CN",
		"models": {
			"moonshot-v1-128k": {
				"id": "moonshot-v1-128k",
				"name": "Moonshot v1 128k",
				"cost": {
					"input": 0.6,
					"output": 1.2
				}
			}
		}
	},
	"alibaba": {
		"id": "alibaba",
		"name": "Alibaba Cloud",
		"models": {
			"qwen-turbo": {
				"id": "qwen-turbo",
				"name": "Qwen Turbo",
				"cost": {
					"input": 0.3,
					"output": 0.6
				}
			}
		}
	},
	"alibaba-cn": {
		"id": "alibaba-cn",
		"name": "Alibaba Cloud China",
		"models": {
			"qwen-max": {
				"id": "qwen-max",
				"name": "Qwen Max",
				"cost": {
					"input": 2.0,
					"output": 6.0
				}
			}
		}
	},
	"tencent": {
		"id": "tencent",
		"name": "Tencent Hunyuan",
		"models": {
			"hunyuan-pro": {
				"id": "hunyuan-pro",
				"name": "Hunyuan Pro",
				"cost": {
					"input": 1.5,
					"output": 4.5
				}
			}
		}
	},
	"volcengine": {
		"id": "volcengine",
		"name": "Volcengine Doubao",
		"models": {
			"doubao-pro-32k": {
				"id": "doubao-pro-32k",
				"name": "Doubao Pro 32k",
				"cost": {
					"input": 0.8,
					"output": 2.0
				}
			}
		}
	},
	"cloudflare-workers-ai": {
		"id": "cloudflare-workers-ai",
		"name": "Cloudflare Workers AI",
		"models": {
			"llama-3-8b-instruct": {
				"id": "llama-3-8b-instruct",
				"name": "Llama 3 8B Instruct",
				"cost": {
					"input": 0.0,
					"output": 0.0
				}
			}
		}
	}
}`

func TestService_3TierResolution(t *testing.T) {
	svc := catalog.NewService(catalog.Config{})
	err := svc.LoadFromBytes([]byte(sampleCatalogJSON))
	require.NoError(t, err)

	// 1. Tier 1: Exact Provider + Model Match
	t.Run("Tier1_ExactMatch", func(t *testing.T) {
		spec, ok := svc.FindModel("google", "gemini-2.5-flash")
		require.True(t, ok)
		assert.Equal(t, "gemini-2.5-flash", spec.ID)
		assert.Equal(t, "google", spec.Provider)

		spec, ok = svc.FindModel("openai", "gpt-4o")
		require.True(t, ok)
		assert.Equal(t, "gpt-4o", spec.ID)
		assert.Equal(t, "openai", spec.Provider)

		spec, ok = svc.FindModel("anthropic", "claude-3-7-sonnet")
		require.True(t, ok)
		assert.Equal(t, "claude-3-7-sonnet", spec.ID)
		assert.Equal(t, "anthropic", spec.Provider)

		// ModelID with provider prefix
		spec, ok = svc.FindModel("", "openai/gpt-4o")
		require.True(t, ok)
		assert.Equal(t, "gpt-4o", spec.ID)
		assert.Equal(t, "openai", spec.Provider)
	})

	// 2. Tier 2: Alias Provider Match
	t.Run("Tier2_AliasMatch", func(t *testing.T) {
		aliasTests := []struct {
			gatewaySlug      string
			modelID          string
			expectedProvider string
			expectedModelID  string
		}{
			{"antigravity", "gemini-2.5-flash", "google", "gemini-2.5-flash"},
			{"cline", "claude-3-7-sonnet", "anthropic", "claude-3-7-sonnet"},
			{"xiaomi-mimo", "mimo-v1", "xiaomi", "mimo-v1"},
			{"glm", "glm-4-plus", "zai", "glm-4-plus"},
			{"glm-cn", "glm-4-air", "zhipuai", "glm-4-air"},
			{"zhipu", "glm-4-air", "zhipuai", "glm-4-air"},
			{"kimi", "moonshot-v1-8k", "moonshotai", "moonshot-v1-8k"},
			{"kimi-cn", "moonshot-v1-128k", "moonshotai-cn", "moonshot-v1-128k"},
			{"qwen", "qwen-turbo", "alibaba", "qwen-turbo"},
			{"qwen-cn", "qwen-max", "alibaba-cn", "qwen-max"},
			{"hunyuan", "hunyuan-pro", "tencent", "hunyuan-pro"},
			{"doubao", "doubao-pro-32k", "volcengine", "doubao-pro-32k"},
			{"cloudflare-ai", "llama-3-8b-instruct", "cloudflare-workers-ai", "llama-3-8b-instruct"},
		}

		for _, tc := range aliasTests {
			t.Run(tc.gatewaySlug+"_"+tc.modelID, func(t *testing.T) {
				spec, ok := svc.FindModel(tc.gatewaySlug, tc.modelID)
				require.True(t, ok, "expected alias %s to resolve", tc.gatewaySlug)
				assert.Equal(t, tc.expectedProvider, spec.Provider)
				assert.Equal(t, tc.expectedModelID, spec.ID)
			})
		}
	})

	// 3. Tier 3: Canonical Base Model Slug Fallback
	t.Run("Tier3_CanonicalFallback", func(t *testing.T) {
		// Unknown provider "cline" or "custom-client" with canonical model "claude-3-7-sonnet"
		spec, ok := svc.FindModel("unknown-client", "claude-3-7-sonnet")
		require.True(t, ok)
		assert.Equal(t, "claude-3-7-sonnet", spec.ID)
		assert.Equal(t, "anthropic", spec.Provider)

		// Model with tags or version suffix
		spec, ok = svc.FindModel("my-custom-gw", "gpt-4o:latest")
		require.True(t, ok)
		assert.Equal(t, "gpt-4o", spec.ID)
		assert.Equal(t, "openai", spec.Provider)

		// Provider with full vendor path
		spec, ok = svc.FindModel("generic-proxy", "google/gemini-2.5-flash")
		require.True(t, ok)
		assert.Equal(t, "gemini-2.5-flash", spec.ID)
		assert.Equal(t, "google", spec.Provider)
	})

	// 4. Missing / Unresolved
	t.Run("NotFound", func(t *testing.T) {
		spec, ok := svc.FindModel("nonexistent", "nonexistent-model")
		assert.False(t, ok)
		assert.Nil(t, spec)

		spec, ok = svc.FindModel("", "")
		assert.False(t, ok)
		assert.Nil(t, spec)
	})
}

func TestService_GetPrice(t *testing.T) {
	svc := catalog.New(catalog.Config{})
	err := svc.LoadFromBytes([]byte(sampleCatalogJSON))
	require.NoError(t, err)

	// Exact match price lookup
	price, ok := svc.GetPrice("google", "gemini-2.5-flash")
	require.True(t, ok)
	assert.Equal(t, 0.3, price.InputPerM)
	assert.Equal(t, 2.5, price.OutputPerM)
	assert.Equal(t, 0.03, price.CachedInputPerM)
	assert.Equal(t, 0.0, price.CacheWritePerM)
	assert.Equal(t, 2.5, price.ReasoningPerM)

	// Alias price lookup ("antigravity" -> "google")
	price, ok = svc.GetPrice("antigravity", "gemini-2.5-flash")
	require.True(t, ok)
	assert.Equal(t, 0.3, price.InputPerM)
	assert.Equal(t, 2.5, price.OutputPerM)

	// Non-existent model price lookup
	price, ok = svc.GetPrice("unknown", "unknown-model")
	assert.False(t, ok)
	assert.Equal(t, meter.Price{}, price)
}

func TestService_ListAllModels(t *testing.T) {
	svc := catalog.NewService(catalog.Config{})
	err := svc.LoadFromBytes([]byte(sampleCatalogJSON))
	require.NoError(t, err)

	models := svc.ListAllModels()
	assert.Len(t, models, 14)

	// Verify sorting order: by Provider then ID
	for i := 1; i < len(models); i++ {
		prev := models[i-1]
		curr := models[i]
		if prev.Provider == curr.Provider {
			assert.True(t, prev.ID <= curr.ID, "expected %s <= %s for provider %s", prev.ID, curr.ID, prev.Provider)
		} else {
			assert.True(t, prev.Provider < curr.Provider, "expected %s < %s", prev.Provider, curr.Provider)
		}
	}

	// Verify returned slice is isolated
	models[0].Name = "Modified Name"
	freshList := svc.ListAllModels()
	assert.NotEqual(t, "Modified Name", freshList[0].Name)
}

func TestService_LoadFromBytes_InvalidJSON(t *testing.T) {
	svc := catalog.New(catalog.Config{})
	err := svc.LoadFromBytes([]byte(`{not valid json}`))
	require.Error(t, err)
}

func TestResolveProviderAlias(t *testing.T) {
	for alias, expected := range catalog.ProviderAliases {
		assert.Equal(t, expected, catalog.ResolveProviderAlias(alias))
		assert.Equal(t, expected, catalog.ResolveProviderAlias("  "+alias+"  "))
	}

	assert.Equal(t, "openai", catalog.ResolveProviderAlias("openai"))
	assert.Equal(t, "custom-provider", catalog.ResolveProviderAlias("CUSTOM-PROVIDER"))
}
