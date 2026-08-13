#!/bin/bash
cd src/

# Catch all remaining brackets [var(--something)]
find . -type f -name "*.tsx" -exec sed -i '' 's/\[var(--text-muted)\]/muted-foreground/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/\[var(--text)\]/foreground/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/\[var(--border)\]/border/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/\[var(--bg-elevated)\]/popover/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/\[var(--bg-subtle)\]/muted/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/\[var(--bg)\]/background/g' {} +

# Fallback fix for leftover classes that might become invalid like `text-muted-foreground` -> it should just be `text-muted-foreground` not `text-[muted-foreground]`
find . -type f -name "*.tsx" -exec sed -i '' 's/text-\[muted-foreground\]/text-muted-foreground/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/text-\[foreground\]/text-foreground/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/bg-\[muted\]/bg-muted/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/bg-\[popover\]/bg-popover/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/bg-\[background\]/bg-background/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/border-\[border\]/border-border/g' {} +
find . -type f -name "*.tsx" -exec sed -i '' 's/ring-\[border\]/ring-border/g' {} +

echo "Done"
