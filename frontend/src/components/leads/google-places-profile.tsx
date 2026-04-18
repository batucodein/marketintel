"use client";

import type { BusinessWithRelevance } from "@/lib/types/business";
import { Badge } from "@/components/ui/badge";
import {
  Globe,
  Phone,
  Mail,
  Star,
  BadgeCheck,
  MapPinOff,
  ExternalLink,
} from "lucide-react";

interface GooglePlacesProfileProps {
  business: BusinessWithRelevance;
}

export function GooglePlacesProfile({ business }: GooglePlacesProfileProps) {
  if (!business.google_place_id) {
    return (
      <div className="flex items-start gap-3 rounded-lg border border-dashed bg-muted/30 px-4 py-3">
        <MapPinOff className="h-5 w-5 text-muted-foreground shrink-0 mt-0.5" />
        <div>
          <p className="text-sm text-muted-foreground">
            No Google Places profile found for this business.
          </p>
          <p className="text-xs text-muted-foreground/70 mt-1">
            Contact information may require manual research.
          </p>
        </div>
      </div>
    );
  }

  const googleTypes = business.google_types ?? business.social_links?.google_places_data?.google_types ?? [];
  const rating = business.rating ?? business.social_links?.google_places_data?.rating ?? null;
  const ratingCount = business.rating_count ?? business.social_links?.google_places_data?.rating_count ?? null;

  return (
    <div className="space-y-3">
      {/* Contact links */}
      <div className="space-y-1.5">
        {business.website && (
          <ContactRow icon={Globe} label="Website">
            <a
              href={business.website}
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm text-blue-600 hover:underline flex items-center gap-1 truncate"
            >
              {business.website.replace(/^https?:\/\//, "").replace(/\/$/, "")}
              <ExternalLink className="h-3 w-3 shrink-0" />
            </a>
          </ContactRow>
        )}
        {business.phone && (
          <ContactRow icon={Phone} label="Phone">
            <span className="text-sm">{business.phone}</span>
          </ContactRow>
        )}
        {business.email && (
          <ContactRow icon={Mail} label="Email">
            <a
              href={`mailto:${business.email}`}
              className="text-sm text-blue-600 hover:underline"
            >
              {business.email}
            </a>
          </ContactRow>
        )}
      </div>

      {/* Rating */}
      {rating != null && (
        <div className="flex items-center gap-2">
          <Star className="h-4 w-4 text-amber-500 fill-amber-500 shrink-0" />
          <span className="text-sm font-semibold">{rating.toFixed(1)}</span>
          {ratingCount != null && (
            <span className="text-xs text-muted-foreground">
              ({ratingCount} reviews)
            </span>
          )}
        </div>
      )}

      {/* Google business types */}
      {googleTypes.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {googleTypes.map((type) => (
            <Badge key={type} variant="secondary" className="text-[10px] text-muted-foreground">
              {type.replace(/_/g, " ")}
            </Badge>
          ))}
        </div>
      )}

      {/* Description */}
      {business.description && (
        <p className="text-sm text-muted-foreground leading-relaxed">
          {business.description}
        </p>
      )}

      {/* Verified indicator */}
      <div className="flex items-center gap-1.5 text-emerald-600">
        <BadgeCheck className="h-4 w-4" />
        <span className="text-xs font-medium">Verified on Google</span>
      </div>
    </div>
  );
}

function ContactRow({
  icon: Icon,
  label,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-center gap-2">
      <Icon className="h-4 w-4 text-muted-foreground shrink-0" />
      <span className="text-xs text-muted-foreground w-14 shrink-0">{label}</span>
      {children}
    </div>
  );
}
