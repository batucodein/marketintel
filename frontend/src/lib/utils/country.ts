const COUNTRY_NAMES: Record<string, string> = {
  US: "United States", DE: "Germany", GB: "United Kingdom", FR: "France",
  IT: "Italy", ES: "Spain", NL: "Netherlands", TR: "Turkey", CN: "China",
  JP: "Japan", KR: "South Korea", IN: "India", BR: "Brazil", RU: "Russia",
  SA: "Saudi Arabia", AE: "UAE", EG: "Egypt", NG: "Nigeria", ZA: "South Africa",
  AU: "Australia", CA: "Canada", MX: "Mexico", AR: "Argentina", CL: "Chile",
  CO: "Colombia", PE: "Peru", SE: "Sweden", NO: "Norway", DK: "Denmark",
  FI: "Finland", PL: "Poland", CZ: "Czechia", AT: "Austria", CH: "Switzerland",
  BE: "Belgium", PT: "Portugal", GR: "Greece", RO: "Romania", HU: "Hungary",
  IE: "Ireland", IL: "Israel", TH: "Thailand", VN: "Vietnam", MY: "Malaysia",
  SG: "Singapore", ID: "Indonesia", PH: "Philippines", PK: "Pakistan",
  BD: "Bangladesh", UA: "Ukraine", KZ: "Kazakhstan", UZ: "Uzbekistan",
  IR: "Iran", IQ: "Iraq", MA: "Morocco", TN: "Tunisia", KE: "Kenya",
  GH: "Ghana", TZ: "Tanzania", ET: "Ethiopia",
};

export function countryFlag(code: string): string {
  const c = code.toUpperCase();
  return String.fromCodePoint(...[...c].map((ch) => ch.charCodeAt(0) + 0x1f1a5));
}

export function countryName(code: string): string {
  return COUNTRY_NAMES[code.toUpperCase()] || code.toUpperCase();
}

const NAME_TO_CODE: Record<string, string> = Object.fromEntries(
  Object.entries(COUNTRY_NAMES).map(([code, name]) => [name.toLowerCase(), code]),
);

/** Accepts either an ISO code ("TR") or a full name ("Turkey") and returns the ISO code. */
export function toCountryCode(input: string): string {
  const upper = input.toUpperCase();
  if (COUNTRY_NAMES[upper]) return upper; // already a code
  return NAME_TO_CODE[input.toLowerCase()] || upper;
}

export const COUNTRY_LIST = Object.entries(COUNTRY_NAMES)
  .map(([code, name]) => ({ code, name }))
  .sort((a, b) => a.name.localeCompare(b.name));
