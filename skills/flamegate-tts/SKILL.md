---
name: flamegate-tts
description: Text-to-speech via FlameGate /v1/audio/speech using OpenAI / ElevenLabs / Deepgram / Edge TTS / Google TTS / Inworld voices. Use when the user wants to convert text to speech, generate audio, voiceover, narrate, or read text aloud.
---

# FlameGate — Text-to-Speech

Requires `FLAMEGATE_URL` (and `FLAMEGATE_KEY` if auth enabled). See https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate/SKILL.md for setup.

## Discover

```bash
# 1) List models
curl $FLAMEGATE_URL/v1/models/tts | jq '.data[].id'
# 2) Per-model metadata (params, voicesUrl if voice-by-id)
curl "$FLAMEGATE_URL/v1/models/info?id=el/eleven_multilingual_v2"
```

`model` field in `/v1/audio/speech` = voice ID directly (e.g. `edge-tts/vi-VN-HoaiMyNeural`, `el/<voice_id>`, or `openai/tts-1` model+default voice).

## Endpoint

`POST $FLAMEGATE_URL/v1/audio/speech`

| Field | Required | Notes |
|---|---|---|
| `model` | yes | voice ID from `/v1/models/tts` |
| `input` | yes | text to speak |

Query `?response_format=mp3` (default, raw bytes) or `?response_format=json` (`{audio: base64, format}`).

## Examples

Save MP3:

```bash
curl -X POST "$FLAMEGATE_URL/v1/audio/speech" \
  -H "Authorization: Bearer $FLAMEGATE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"openai/tts-1","input":"Hello world"}' \
  --output speech.mp3
```

JS (save file):

```js
import { writeFile } from "node:fs/promises";
const r = await fetch(`${process.env.FLAMEGATE_URL}/v1/audio/speech`, {
  method: "POST",
  headers: { "Authorization": `Bearer ${process.env.FLAMEGATE_KEY}`, "Content-Type": "application/json" },
  body: JSON.stringify({ model: "el/eleven_multilingual_v2", input: "Hello world" }),
});
await writeFile("speech.mp3", Buffer.from(await r.arrayBuffer()));
```

## Response shape

Default → raw audio bytes (Content-Type `audio/mp3`).

`?response_format=json`:
```json
{ "audio": "SUQzBAAAA...", "format": "mp3" }
```

## Provider quick reference

| Provider | `model` format | Notes |
|---|---|---|
| OpenAI | `tts-1/alloy` (model/voice) or just voice | Default model `gpt-4o-mini-tts` |
| ElevenLabs | `<model_id>/<voice_id>` or `<voice_id>` | Default model `eleven_flash_v2_5` |
| Edge TTS | voice id e.g. `vi-VN-HoaiMyNeural` | **noAuth**; free |
| Deepgram | `aura-asteria-en` etc | Token auth |
| Inworld | `model/voice` | Provider-specific auth |
