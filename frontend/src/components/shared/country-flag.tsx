import { countryFlag, countryName } from "@/lib/utils/country";

interface CountryFlagProps {
  code: string;
  showName?: boolean;
}

export function CountryFlag({ code, showName = true }: CountryFlagProps) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="text-base leading-none">{countryFlag(code)}</span>
      {showName && <span>{countryName(code)}</span>}
    </span>
  );
}
