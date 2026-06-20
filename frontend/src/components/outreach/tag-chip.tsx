import { Badge } from "@/components/ui/badge";
import { X } from "lucide-react";
import { tagLabel, tagStyle } from "./intent-tags";

// TagChip renders one intent tag, coloured by its class. Pass onRemove to show a
// removable × (used by the manual tag editor).
export function TagChip({
  tag,
  onRemove,
  className = "",
}: {
  tag: string;
  onRemove?: () => void;
  className?: string;
}) {
  return (
    <Badge
      variant="outline"
      className={`text-[10px] whitespace-nowrap shrink-0 gap-1 ${tagStyle(tag)} ${className}`}
    >
      {tagLabel(tag)}
      {onRemove && (
        <button
          type="button"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onRemove();
          }}
          className="ml-0.5 rounded-sm hover:opacity-70 cursor-pointer"
          aria-label={`Remove ${tagLabel(tag)}`}
        >
          <X className="h-2.5 w-2.5" />
        </button>
      )}
    </Badge>
  );
}
