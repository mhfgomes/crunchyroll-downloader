import { NumberField } from "@base-ui/react/number-field"
import { MinusIcon, PlusIcon } from "lucide-react"

const button =
  "flex size-8 items-center justify-center text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-40"

/** A compact stepper: minus, value, plus. */
export function NumberStepper({
  value,
  onChange,
  min,
  max,
  suffix,
  label,
}: {
  value: number
  onChange: (v: number) => void
  min?: number
  max?: number
  suffix?: string
  label: string
}) {
  return (
    <NumberField.Root
      value={value}
      min={min}
      max={max}
      onValueChange={(v) => onChange(v ?? min ?? 0)}
      className="flex items-center gap-2.5"
    >
      <NumberField.Group className="flex h-8 items-center overflow-hidden rounded-lg border border-input bg-input/30 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50">
        <NumberField.Decrement className={button} aria-label={`Decrease ${label}`}>
          <MinusIcon className="size-3.5" />
        </NumberField.Decrement>
        <NumberField.Input
          aria-label={label}
          className="h-full w-12 border-x border-input bg-transparent text-center text-sm tabular-nums outline-none"
        />
        <NumberField.Increment className={button} aria-label={`Increase ${label}`}>
          <PlusIcon className="size-3.5" />
        </NumberField.Increment>
      </NumberField.Group>
      {suffix && <span className="text-sm text-muted-foreground">{suffix}</span>}
    </NumberField.Root>
  )
}
