import { useTheme } from "next-themes"
import { Toaster as Sonner, type ToasterProps } from "sonner"
import { CircleCheckIcon, InfoIcon, TriangleAlertIcon, OctagonXIcon, Loader2Icon } from "lucide-react"

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme()

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      className="toaster group"
      closeButton={true}
      icons={{
        success: (
          <div className="flex shrink-0 items-center justify-center rounded-full bg-emerald-500/15 text-emerald-400 p-1.5 border border-emerald-500/25 shadow-[0_0_12px_rgba(16,185,129,0.2)]">
            <CircleCheckIcon className="size-3.5" />
          </div>
        ),
        info: (
          <div className="flex shrink-0 items-center justify-center rounded-full bg-blue-500/15 text-blue-400 p-1.5 border border-blue-500/25 shadow-[0_0_12px_rgba(59,130,246,0.2)]">
            <InfoIcon className="size-3.5" />
          </div>
        ),
        warning: (
          <div className="flex shrink-0 items-center justify-center rounded-full bg-amber-500/15 text-amber-400 p-1.5 border border-amber-500/25 shadow-[0_0_12px_rgba(245,158,11,0.2)]">
            <TriangleAlertIcon className="size-3.5" />
          </div>
        ),
        error: (
          <div className="flex shrink-0 items-center justify-center rounded-full bg-rose-500/15 text-rose-400 p-1.5 border border-rose-500/25 shadow-[0_0_12px_rgba(244,63,94,0.25)]">
            <OctagonXIcon className="size-3.5" />
          </div>
        ),
        loading: (
          <div className="flex shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary p-1.5 border border-primary/20">
            <Loader2Icon className="size-3.5 animate-spin" />
          </div>
        ),
      }}
      toastOptions={{
        classNames: {
          toast: "group rounded-2xl !p-4 !flex !gap-3.5 border backdrop-blur-xl font-sans text-sm select-none transition-all duration-300 w-[380px] shadow-2xl !items-start",
          content: "flex flex-col justify-center flex-1 !pr-6",
          title: "text-[13px] font-bold tracking-tight text-foreground leading-none",
          description: "text-xs text-muted-foreground/90 leading-relaxed mt-1.5 whitespace-pre-wrap break-words",
          success: "!bg-emerald-950/20 dark:!bg-emerald-950/30 !border-emerald-500/20 !text-emerald-200 [box-shadow:0_12px_40px_rgba(16,185,129,0.06),inset_0_0_12px_rgba(16,185,129,0.02)]",
          error: "!bg-rose-950/25 dark:!bg-rose-950/35 !border-rose-500/20 !text-rose-200 [box-shadow:0_12px_40px_rgba(244,63,94,0.08),inset_0_0_12px_rgba(244,63,94,0.03)]",
          warning: "!bg-amber-950/20 dark:!bg-amber-950/30 !border-amber-500/20 !text-amber-200 [box-shadow:0_12px_40px_rgba(245,158,11,0.06),inset_0_0_12px_rgba(245,158,11,0.02)]",
          info: "!bg-blue-950/20 dark:!bg-blue-950/30 !border-blue-500/20 !text-blue-200 [box-shadow:0_12px_40px_rgba(59,130,246,0.06),inset_0_0_12px_rgba(59,130,246,0.02)]",
          actionButton: "!bg-primary !text-primary-foreground !rounded-lg !font-medium !text-xs !px-3.5 !py-1.5 !transition-colors hover:!bg-primary/90 !ml-auto !mt-auto !mb-auto !shrink-0",
          cancelButton: "!bg-muted !text-muted-foreground !rounded-lg !font-medium !text-xs !px-3.5 !py-1.5 !transition-colors hover:!bg-muted/80 !ml-auto !mt-auto !mb-auto !shrink-0",
          closeButton: "!absolute !top-4 !right-3 !left-auto !translate-x-0 !translate-y-0 !bg-transparent hover:!bg-white/10 !border-transparent hover:!text-foreground !text-muted-foreground/60 !rounded-lg !p-1 !h-6 !w-6 !flex !items-center !justify-center !transition-all cursor-pointer z-50",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
