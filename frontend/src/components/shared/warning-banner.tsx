import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertTriangle } from "lucide-react";

interface WarningBannerProps {
  warnings: string[];
}

export function WarningBanner({ warnings }: WarningBannerProps) {
  if (!warnings.length) return null;

  return (
    <Alert className="border-amber-200 bg-amber-50 text-amber-800">
      <AlertTriangle className="h-4 w-4 text-amber-600" />
      <AlertDescription>
        <ul className="space-y-1">
          {warnings.map((w, i) => (
            <li key={i} className="text-sm">{w}</li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}
