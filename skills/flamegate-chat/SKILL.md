---
name: flamegate-chat
description: Chat / code generation via FlameGate using OpenAI /v1/chat/completions or Anthropic /v1/messages format with streaming + auto-fallback combos. Use when the user wants to ask an LLM, generate code, summarize text, or run prompts through FlameGate.
---

# FlameGate — Chat

Requires `FLAMEGATE_URL` (and `FLAMEGATE_KEY` if auth enabled). See https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate/SKILL.md for setup.

## Endpoints

- `POST $FLAMEGATE_URL/v1/chat/completions` — OpenAI format
- `POST $FLAMEGATE_URL/v1/messages` — Anthropic format
- `POST $FLAMEGATE_URL/v1/responses` — OpenAI Responses format

## Discover

```bash
curl $FLAMEGATE_URL/v1/models | jq '.data[].id'
# Per-model metadata (contextWindow, params)
curl "$FLAMEGATE_URL/v1/models/info?id=openai/gpt-4o"
```

Combos (e.g. `vip`, `mycodex`) auto-fallback through multiple providers.

## OpenAI format

```bash
curl -X POST $FLAMEGATE_URL/v1/chat/completions \
  -H "Authorization: Bearer $FLAMEGATE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"openai/gpt-5","messages":[{"role":"user","content":"Hi"}],"stream":false}'
```

JS (OpenAI SDK):

```js
import OpenAI from "openai";
const client = new OpenAI({ baseURL: `${process.env.FLAMEGATE_URL}/v1`, apiKey: process.env.FLAMEGATE_KEY });
const res = await client.chat.completions.create({
  model: "openai/gpt-5",
  messages: [{ role: "user", content: "Hi" }],
  stream: true,
});
for await (const chunk of res) process.stdout.write(chunk.choices[0]?.delta?.content || "");
```

## Anthropic format

```bash
curl -X POST $FLAMEGATE_URL/v1/messages \
  -H "Authorization: Bearer $FLAMEGATE_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{"model":"anthropic/claude-opus-4-7","max_tokens":1024,"messages":[{"role":"user","content":"Hi"}]}'
```

## Response shape

OpenAI (`/v1/chat/completions`):
```json
{ "id": "chatcmpl-...", "object": "chat.completion", "model": "openai/gpt-5",
  "choices": [{ "index": 0, "message": { "role": "assistant", "content": "Hello!" }, "finish_reason": "stop" }],
  "usage": { "prompt_tokens": 8, "completion_tokens": 2, "total_tokens": 10 } }
```

Streaming (`stream:true`) emits SSE: `data: {choices:[{delta:{content:"..."}}]}\n\n` ... `data: [DONE]\n\n`.

Anthropic (`/v1/messages`):
```json
{ "id": "msg_...", "type": "message", "role": "assistant", "model": "anthropic/claude-opus-4-7",
  "content": [{ "type": "text", "text": "Hello!" }],
  "stop_reason": "end_turn", "usage": { "input_tokens": 8, "output_tokens": 2 } }
```

## Provider quick reference

| Provider | `model` format | Examples |
|---|---|---|
| OpenAI | `openai/<model>` | `openai/gpt-5`, `openai/gpt-4o` |
| Anthropic | `anthropic/<model>` | `anthropic/claude-opus-4-7`, `anthropic/claude-sonnet-4-6` |
| Claude Code | `cc/<model>` | `cc/claude-opus-4-7` |
| Gemini | `gemini/<model>` | `gemini/gemini-2.5-pro`, `gemini/gemini-2.5-flash` |
| Groq | `groq/<model>` | `groq/llama-3.3-70b` |
| DeepSeek | `ds/<model>` | `ds/deepseek-chat`, `ds/deepseek-coder` |
| OpenRouter | `openrouter/<model>` | `openrouter/anthropic/claude-opus-4-7` |
| Mistral | `mistral/<model>` | `mistral/mistral-large-latest` |
| xAI | `xai/<model>` | `xai/grok-3` |
| Ollama Local | `ollama-local/<model>` | `ollama-local/llama3.2` |
| Custom OpenAI | `custom-openai/<model>` | Any model on your custom endpoint |
