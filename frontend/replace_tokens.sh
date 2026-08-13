#!/bin/bash

# Replaces old FlameGate tokens with standard Shadcn/Ignition tokens
# --bg-elevated -> bg-surface-container-low OR bg-popover
# --bg-subtle -> bg-surface-container
# --bg -> bg-background
# --border -> border-border
# --text-muted -> text-muted-foreground
# --text -> text-foreground

cd src/

# 1. Text Muted
# Replace text-[var(--text-muted)] -> text-muted-foreground
find . -type f -name "*.tsx" -exec sed -i '' 's/text-\[var(--text-muted)\]/text-muted-foreground/g' {} +

# 2. Text Foreground
# Replace text-[var(--text)] -> text-foreground
find . -type f -name "*.tsx" -exec sed -i '' 's/text-\[var(--text)\]/text-foreground/g' {} +

# 3. Border 
# Replace border-[var(--border)] -> border-border 
find . -type f -name "*.tsx" -exec sed -i '' 's/border-\[var(--border)\]/border-border/g' {} +

# 4. Backgrounds
# Because Shadcn provides bg-background, bg-muted
find . -type f -name "*.tsx" -exec sed -i '' 's/bg-\[var(--bg)\]/bg-background/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/bg-\[var(--bg-elevated)\]/bg-popover/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/bg-\[var(--bg-subtle)\]/bg-muted/g' {} +

# 5. Shadows
find . -type f -name "*.tsx" -exec sed -i '' 's/shadow-\[var(--shadow-pop)\]/shadow-md/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/shadow-\[var(--shadow-float)\]/shadow-lg/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/shadow-\[var(--shadow-card)\]/shadow-sm/g' {} +

echo "Tokens replacement completed."
