"use client";

import { cn } from "@/lib/utils";

export type AutomationLevel = "manual" | "semi" | "auto";

const OPTIONS: { value: AutomationLevel; label: string; help: string }[] = [
  { value: "manual", label: "Manual", help: "No AI drafts unless you ask" },
  { value: "semi", label: "Semi", help: "AI drafts replies; you approve before send" },
  { value: "auto", label: "Auto", help: "AI drafts AND sends without approval (sequences)" },
];

export interface Props {
  value: AutomationLevel;
  onChange: (next: AutomationLevel) => void | Promise<void>;
  disabled?: boolean;
}

export function AutomationToggle({ value, onChange, disabled }: Props) {
  return (
    <div className="inline-flex rounded-md border border-input bg-background p-0.5 text-xs">
      {OPTIONS.map((opt) => (
        <button
          key={opt.value}
          onClick={() => !disabled && opt.value !== value && onChange(opt.value)}
          disabled={disabled}
          title={opt.help}
          className={cn(
            "px-2 py-1 rounded transition-colors",
            value === opt.value
              ? "bg-blue-600 text-white"
              : "text-muted-foreground hover:text-foreground",
            disabled && "opacity-50 cursor-not-allowed",
          )}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}
