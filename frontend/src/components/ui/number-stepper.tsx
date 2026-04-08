import { Minus, Plus } from "lucide-react"
import { Button } from "./button"
import { Input } from "./input"
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
    <div className={cn("relative group flex items-center w-full", className)}>
      <Input
        type="number"
        value={numericValue}
        onChange={(e) => {
          const val = Number(e.target.value);
          if (!isNaN(val)) onChange(val);
        }}
        className={cn(
          "h-9 w-full pr-24 font-bold tabular-nums focus-visible:ring-1 bg-background/50 border-muted-foreground/20",
          className
        )}
        disabled={disabled}
        min={min}
        max={max}
      />

      <div className="absolute right-1 flex items-center gap-1">
        {unit && (
          <span className="text-[10px] font-black text-muted-foreground/40 uppercase pointer-events-none mr-2">
            {unit}
          </span>
        )}
        
        <div className="flex bg-muted/30 rounded-md p-0.5 border border-muted-foreground/10">
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
          
          <div className="w-[1px] h-4 bg-muted-foreground/10 self-center" />

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
    </div>
  );
}
