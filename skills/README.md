# FlameGate — Agent Skills

Drop-in skills for any AI agent (Claude, Cursor, ChatGPT, custom SDK). Just **copy a link** below and paste it to your AI — it will fetch the skill and use FlameGate for you.

> Tip: start with the **flamegate** entry skill — it covers setup and links to all capability skills.

## Skills

| Capability | Copy link below and paste to your AI |
|---|---|
| **Entry / Setup** (start here) | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate/SKILL.md |
| Chat / code-gen | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-chat/SKILL.md |
| Image generation | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-image/SKILL.md |
| Text-to-speech | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-tts/SKILL.md |
| Speech-to-text | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-stt/SKILL.md |
| Embeddings | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-embeddings/SKILL.md |
| Web search | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-web-search/SKILL.md |
| Web fetch (URL → markdown) | https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate-web-fetch/SKILL.md |

## How to use

Paste to your AI (Claude, Cursor, ChatGPT, …):

```
Read this skill and use it: https://raw.githubusercontent.com/mydisha/flamegate/main/skills/flamegate/SKILL.md
```

Then ask normally — *"generate an image of a cat"*, *"transcribe this URL"*, etc.

## Configure your shell once

```bash
export FLAMEGATE_URL="http://localhost:20180"   # local default, or your VPS / tunnel URL
export FLAMEGATE_KEY="sk-..."                   # from Dashboard → Keys (only if auth enabled)
```

Verify: `curl $FLAMEGATE_URL/healthz` → `{"ok":true}`.

## Links

- Source: https://github.com/mydisha/flamegate
- Dashboard: http://localhost:20180 (local)
