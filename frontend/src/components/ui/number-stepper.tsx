import { Minus, Plus } from "lucide-react"
import { Button } from "./button"
import { cn } from "@/lib/utils"

interface NumberStepperProps {
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
  step?: number;
  className?: string;
  unit?: string;
  disabled?: boolean;
}

export function NumberStepper({
  value,
  onChange,
  min = 0,
  max = 1000,
  step = 1,
  className,
  unit,
  disabled = false
}: NumberStepperProps) {
  const numericValue = Number(value) || 0;

  const handleDecrement = () => {
    if (numericValue > min) {
      onChange(Math.max(min, numericValue - step));
    }
  };

  const handleIncrement = () => {
    if (numericValue < max) {
      onChange(Math.min(max, numericValue + step));
    }
  };

  return (
    <div className={cn("flex h-12 w-full items-center rounded-lg border border-muted-foreground/20 bg-background/50 px-3 transition-all focus-within:ring-1 focus-within:ring-ring", className)}>
      <div className="shrink-0">
        <input
          type="number"
          value={numericValue}
          onChange={(e) => {
            const val = Number(e.target.value);
            if (!isNaN(val)) onChange(val);
          }}
          className="block h-5 w-10 bg-transparent text-base font-bold tabular-nums outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
          disabled={disabled}
          min={min}
          max={max}
        />
        {unit && (
          <span className="block text-[8px] font-bold uppercase tracking-widest text-muted-foreground/50 select-none">
            {unit}
          </span>
        )}
      </div>

      <div className="flex-1" />

      <div className="flex h-7 shrink-0 items-center rounded-md border border-muted-foreground/10 bg-muted/30 p-px">
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="h-6 w-6 rounded-sm hover:bg-background hover:text-rose-500 transition-colors"
          onClick={handleDecrement}
          disabled={disabled || numericValue <= min}
        >
          <Minus className="h-3 w-3" />
        </Button>

        <div className="h-3 w-px bg-muted-foreground/15" />

        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="h-6 w-6 rounded-sm hover:bg-background hover:text-emerald-500 transition-colors"
          onClick={handleIncrement}
          disabled={disabled || numericValue >= max}
        >
          <Plus className="h-3 w-3" />
        </Button>
      </div>
    </div>
  );
}
