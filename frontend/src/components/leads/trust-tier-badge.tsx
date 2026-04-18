import { Badge } from "@/components/ui/badge";
import { TRUST_TIER_CONFIG } from "@/lib/utils/trust-tier";
import type { TrustTier } from "@/lib/types/business";
import { cn } from "@/lib/utils";
import { BadgeCheck } from "lucide-react";

interface TrustTierBadgeProps {
  tier: TrustTier;
  className?: string;
}

export function TrustTierBadge({ tier, className }: TrustTierBadgeProps) {
  const config = TRUST_TIER_CONFIG[tier] || TRUST_TIER_CONFIG.inferred;
  const isVerified = tier.startsWith("verified");
  return (
    <Badge className={cn(config.bgClass, config.textColor, "text-xs font-medium", className)}>
      {isVerified && <BadgeCheck className="mr-1 h-3 w-3" />}
      {config.label}
    </Badge>
  );
}
