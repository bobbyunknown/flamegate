---
name: flamegate
description: Entry point for FlameGate — local/remote AI gateway with OpenAI-compatible REST for chat, image, TTS, embeddings, web search, web fetch. Use when the user mentions FlameGate, FLAMEGATE_URL, or wants AI without writing provider boilerplate. This skill covers setup + indexes capability skills; fetch the relevant capability SKILL.md from the URLs below when needed.
---

# FlameGate

Local/remote AI gateway exposing OpenAI-compatible REST. One key, many providers, auto-fallback.

## Setup

```bash
export FLAMEGATE_URL="http://localhost:20180"      # or VPS / tunnel URL
export FLAMEGATE_KEY="sk-..."                      # from Dashboard → Keys (only if auth enabled)
```

All requests: `${FLAMEGATE_URL}/v1/...` with header `Authorization: Bearer ${FLAMEGATE_KEY}` (omit if auth disabled).

Verify: `curl $FLAMEGATE_URL/healthz` → `{"ok":true}`

## Discover models

```bash
curl $FLAMEGATE_URL/v1/models                  # chat/LLM (default)
curl $FLAMEGATE_URL/v1/models/image            # image-gen
curl $FLAMEGATE_URL/v1/models/tts              # text-to-speech
curl $FLAMEGATE_URL/v1/models/embedding        # embeddings
curl $FLAMEGATE_URL/v1/models/web              # web search + fetch (entries have `kind` field)
curl $FLAMEGATE_URL/v1/models/stt              # speech-to-text
```

Use `data[].id` as `model` field in requests. Combos appear with `owned_by:"combo"`.

Response shape:
```json
{ "object": "list", "data": [
  { "id": "openai/gpt-5", "object": "model", "owned_by": "openai", "created": 1735000000 },
  { "id": "tavily/search", "object": "model", "kind": "webSearch", "owned_by": "tavily", "created": 1735000000 }
]}
```

## Capability skills

When the user needs a specific capability, fetch that skill's `SKILL.md` from its raw URL:

| Capability | Raw URL |
|---|---|
| Chat / code-gen | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-chat/SKILL.md |
| Image generation | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-image/SKILL.md |
| Text-to-speech | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-tts/SKILL.md |
| Speech-to-text | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-stt/SKILL.md |
| Embeddings | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-embeddings/SKILL.md |
| Web search | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-web-search/SKILL.md |
| Web fetch (URL → markdown) | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-web-fetch/SKILL.md |

## Supported providers (highlights)

| Provider | ID | Alias | Notes |
|---|---|---|---|
| OpenAI | `openai` | `openai` | GPT-5, GPT-4o, DALL-E, Whisper, TTS |
| Anthropic | `anthropic` | `anthropic` | Claude Opus, Sonnet, Haiku |
| Claude Code | `claude` | `cc` | Claude via Claude Code subscription |
| Google Gemini | `gemini` | `gemini` | Gemini 2.5, Imagen |
| Groq | `groq` | `groq` | Fast inference, Whisper |
| DeepSeek | `deepseek` | `ds` | DeepSeek V3, Coder |
| OpenRouter | `openrouter` | `openrouter` | 100+ models via single key |
| Mistral | `mistral` | `mistral` | Mistral Large, Medium |
| xAI | `xai` | `xai` | Grok models |
| NVIDIA NIM | `nvidia` | `nvidia` | Nemotron, Llama |
| Ollama Local | `ollama-local` | `ollama-local` | Local models, no auth |
| Custom OpenAI | `custom-openai` | `custom-openai` | Any OpenAI-compatible endpoint |
| Custom Anthropic | `custom-anthropic` | `custom-anthropic` | Any Anthropic-compatible endpoint |

Use `provider/model` format: `openai/gpt-5`, `anthropic/claude-opus-4-7`, `groq/whisper-large-v3`.

## Errors

- 401 → set/refresh `FLAMEGATE_KEY` (Dashboard → Keys)
- 400 `Invalid model format` → check `model` exists in `/v1/models/<kind>`
- 503 `All accounts unavailable` → wait `retry-after` or add another provider account
- 429 `Budget exhausted` → budget limit reached, check Dashboard → Budgets
