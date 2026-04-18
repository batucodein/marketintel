# MarketIntel V2 Backlog

Feature enhancements to consider after the Excel-only V1 is stable.

## 1. Multi-origin country support

**Current (V1):** We assume each Excel upload has one dominant origin country (≥85% of shipments). The market card shows "Turkey → US". An AI sanity check flags cases where the assumption breaks.

**V2:** Excels from importers' perspectives (e.g. "all suppliers who shipped marble to Acme Corp") genuinely have many origins: Turkey, Italy, China, Spain. No single origin dominates.

**Work needed:**
- Store a full origin distribution on the market (top N with percentages)
- Market card UI: show stacked flags or "3 top origins" badge
- Lead filter: "show leads from Turkish suppliers only"
- Backend: relax the single-origin assumption in `deriveProductName` prompt
- Frontend: new component showing origin breakdown pie/bar chart

## 2. Merge duplicate uploads

**Current (V1):** Uploading the same product+destination twice creates two independent markets. User sees duplicates.

**V2:** Auto-merge if HS code + destination country match an existing market within N days.

**Work needed:**
- Define merge key (dominant_hs_code + destination_country + user_id)
- Union new shipments into existing market, extend date range, refresh derived metadata
- Preserve existing lead scores unless business data meaningfully changed
- Add conflict resolution for contact info (newer source wins, etc.)
- UI: show "merged from N uploads" indicator on market card
- Option to force-create-new instead of merging

## 3. Optional market analysis (re-enable `analyze.go` if we get trade-data)

**Current (V1):** Removed entirely because without Comtrade, analysis was hallucinated.

**V2:** If we integrate a real trade-data source (paid Comtrade, ITC Trade Map, Panjiva, etc.), re-enable market analysis with proper grounding.

**Work needed:**
- Pick a data source and integrate it
- Re-add `analyze.go` prompt with stricter `data_completeness` cap
- Market detail page: AI analysis card behind a feature flag
- Cost tracking: attribute market analysis AI spend separately

## 4. Split markets by HS code group

**Current (V1):** All HS codes in one upload stay in one market. Good when codes are related (6802.91 marble + 6802.93 granite). Bad when codes are unrelated.

**V2:** Detect when HS codes in a single upload belong to distinct product families (e.g. raw materials + finished goods) and offer post-hoc split.

**Work needed:**
- HS code classification (first 4 digits = heading) as split key
- Post-hoc UI: "This market contains 3 distinct product groups. Split?"
- Backend: atomic split-market operation (new markets inherit subset of businesses + lead scores)

## 5. Firecrawl website enrichment

**Current (V1):** Basic HTTP GET + regex extraction. Misses JS-rendered sites, only probes 7 fixed paths.

**V2:** Replace `emailscraper.go` Scraper with Firecrawl (self-hosted or API).

**Work needed:**
- Firecrawl server or API key
- Update `WebsiteScraper` to call Firecrawl
- Richer extraction: product catalogs, team pages, certifications
- Cost tracking: Firecrawl credits

## 6. Cross-discovery lead intelligence

**Current (V1):** Each discovery is independent. No tracking of companies appearing across multiple uploads.

**V2:** Dashboard widget: "These 12 companies appear in 3+ of your discoveries — highest-priority leads."

**Work needed:**
- SQL query: businesses grouped across user's markets
- UI: cross-discovery ranking table
- Maybe trigger re-scoring when a business appears in a new market

## 7. Firecrawl + knowledge graph for supplier networks

**Longer-term:** Build a graph of supplier→importer relationships from customs data. Community detection to find clusters. Lets users ask "show me US marble importers who already buy from Turkish competitors."

## 8. Email marketing & outreach

Already in `PRODUCT_BRIEF.md` as a feature — not implemented yet. Personalized email generation, sending, tracking.
