import { Delete } from "lucide-react"

type PinTone = "blue" | "cyan"

const toneClasses: Record<PinTone, string> = {
  blue: "bg-blue-500 border-blue-500",
  cyan: "bg-cyan-500 border-cyan-500",
}

export function PinDots({
  filled,
  error,
  tone = "cyan",
  className = "my-5 justify-center",
}: {
  filled: number
  error: boolean
  tone?: PinTone
  className?: string
}) {
  return (
    <div className={`flex items-center gap-3 ${className}`}>
      {Array(6).fill(null).map((_, i) => (
        <div
          key={i}
          className={`w-3.5 h-3.5 rounded-full border-2 transition-all duration-150 ${
            error
              ? "bg-red-500 border-red-500"
              : i < filled
                ? `${toneClasses[tone]} scale-110`
                : "border-muted-foreground/25"
          }`}
        />
      ))}
    </div>
  )
}

export function PinKeypad({
  onKey,
  disabled,
}: {
  onKey: (key: string) => void
  disabled: boolean
}) {
  const keys = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "", "0", "del"]
  return (
    <div className="grid grid-cols-3 gap-2.5">
      {keys.map((key, index) => {
        if (key === "") return <div key={index} />
        return (
          <button
            key={index}
            type="button"
            disabled={disabled}
            onClick={() => onKey(key)}
            className="h-14 rounded-xl border border-border bg-background text-lg font-semibold text-foreground transition-all hover:bg-secondary active:scale-95 disabled:cursor-not-allowed disabled:opacity-40 flex items-center justify-center select-none"
            aria-label={key === "del" ? "Delete digit" : `Digit ${key}`}
          >
            {key === "del" ? <Delete className="w-5 h-5 text-muted-foreground" /> : key}
          </button>
        )
      })}
    </div>
  )
}
