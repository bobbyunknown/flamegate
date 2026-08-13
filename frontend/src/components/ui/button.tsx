import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "group/button inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-md border text-xs font-medium transition-all duration-150 outline-none select-none focus-visible:ring-2 focus-visible:ring-ring/30 disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 cursor-pointer",
  {
    variants: {
      variant: {
        default:
          "border-primary/60 bg-primary text-primary-foreground font-semibold shadow-xs hover:bg-primary/90 hover:shadow-sm active:scale-[0.98]",
        secondary:
          "border-border/80 bg-secondary text-secondary-foreground hover:bg-secondary/70 hover:border-border active:scale-[0.98]",
        outline:
          "border-border bg-background text-foreground hover:bg-muted/70 hover:border-foreground/30 active:scale-[0.98]",
        ghost:
          "border-transparent bg-transparent shadow-none text-muted-foreground hover:bg-muted hover:text-foreground active:scale-[0.98]",
        destructive:
          "border-destructive/50 bg-destructive text-destructive-foreground font-semibold shadow-xs hover:bg-destructive/90 active:scale-[0.98]",
        destructiveOutline:
          "border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20 hover:border-destructive/50 active:scale-[0.98]",
        destructiveGhost:
          "border-transparent bg-transparent text-destructive/80 hover:bg-destructive/15 hover:text-destructive active:scale-[0.98]",
        link: "border-transparent bg-transparent shadow-none text-primary underline-offset-4 hover:underline p-0 h-auto",
        noShadow: "border-border bg-background text-foreground hover:bg-muted",
        neutral: "border-border bg-secondary text-secondary-foreground hover:bg-secondary/80",
        reverse: "border-border bg-primary text-primary-foreground hover:bg-primary/90",
      },
      size: {
        default: "h-9 px-4 py-2",
        xs: "h-7 gap-1 px-2.5 text-[11px] [&_svg:not([class*='size-'])]:size-3",
        sm: "h-8 gap-1.5 px-3 text-xs [&_svg:not([class*='size-'])]:size-3.5",
        lg: "h-10 px-5 text-sm font-semibold",
        icon: "size-9",
        "icon-xs": "size-6 [&_svg:not([class*='size-'])]:size-3",
        "icon-sm": "size-7.5 [&_svg:not([class*='size-'])]:size-3.5",
        "icon-lg": "size-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
  return (
    <ButtonPrimitive
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
