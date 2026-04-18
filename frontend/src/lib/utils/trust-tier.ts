import type { TrustTier } from "../types/business";

export const TRUST_TIER_CONFIG: Record<
  TrustTier,
  { label: string; color: string; textColor: string; bgClass: string }
> = {
  verified_buyer: {
    label: "Verified Buyer",
    color: "#064e3b",
    textColor: "text-white",
    bgClass: "bg-emerald-900",
  },
  verified_importer: {
    label: "Verified Importer",
    color: "#065f46",
    textColor: "text-white",
    bgClass: "bg-emerald-800",
  },
  verified: {
    label: "Verified",
    color: "#0d9488",
    textColor: "text-white",
    bgClass: "bg-teal-600",
  },
  confirmed_buyer: {
    label: "Confirmed Buyer",
    color: "#065f46",
    textColor: "text-white",
    bgClass: "bg-emerald-700",
  },
  confirmed_importer: {
    label: "Confirmed Importer",
    color: "#059669",
    textColor: "text-white",
    bgClass: "bg-emerald-500",
  },
  potential: {
    label: "Potential",
    color: "#2563eb",
    textColor: "text-white",
    bgClass: "bg-blue-500",
  },
  inferred: {
    label: "Inferred",
    color: "#9ca3af",
    textColor: "text-gray-800",
    bgClass: "bg-gray-300",
  },
};
