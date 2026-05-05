// Package discovery — canonical.go is the single source of truth for the
// fields MarketIntel knows how to ingest from a customs/CRM Excel file.
//
// The AI mapper takes raw uploaded headers and tries to assign each one to
// a Key in this catalog. parseRowDynamic reads each row using whatever
// header→Key mapping the user confirmed. The AffectsDimension annotation
// is later read by the scoring layer to mechanically cap a dimension's
// sub-score when its inputs were entirely absent for that lead.
package discovery

// Scoring dimension identifiers used by the LeadScore output and by the
// AffectsDimension annotation below. Keep aligned with the keys the AI
// emits in score.go (deal_size, purchase_likelihood, accessibility, fit,
// urgency).
const (
	DimDealSize           = "deal_size"
	DimPurchaseLikelihood = "purchase_likelihood"
	DimAccessibility      = "accessibility"
	DimFit                = "fit"
	DimUrgency            = "urgency"
)

// FieldDef describes one canonical field MarketIntel can store and reason
// about. Adding a new field is a matter of appending a row here plus
// teaching parseRowDynamic where to put the value.
type FieldDef struct {
	// Key is the stable identifier persisted in DB (input_field_presence,
	// search_column_mappings.mapping values). Don't rename without a
	// migration.
	Key string `json:"key"`

	// Label is the human-readable name shown in the mapping UI.
	Label string `json:"label"`

	// Group buckets fields in the UI ("buyer", "shipper", "trade",
	// "logistics") — purely cosmetic.
	Group string `json:"group"`

	// Required fields must map to SOME header for the upload to proceed.
	// The mapping UI surfaces missing required fields and blocks "Import"
	// until they're resolved.
	Required bool `json:"required"`

	// AffectsDimension, when set, ties this field to a scoring dimension.
	// If every field tagged with the same dimension is missing from a
	// lead's input_field_presence AND not filled by enrichment, the
	// scoring layer caps that dimension at 30. Empty = field doesn't
	// influence any dimension's data-quality cap.
	AffectsDimension string `json:"affects_dimension,omitempty"`

	// Description is the short hint shown next to the field in the
	// mapping UI and fed to the AI mapper as part of its prompt.
	Description string `json:"description,omitempty"`
}

// Canonical is the ordered catalog. Order is preserved into the API
// response so the mapping UI renders in a stable, scannable sequence.
var Canonical = []FieldDef{
	// --- Buyer (the lead) ----------------------------------------------
	{Key: "consignee_name", Label: "Buyer / Consignee name", Group: "buyer", Required: true,
		Description: "Importing company. Synonyms: Consignee, Buyer, Importer, To"},
	{Key: "consignee_country", Label: "Buyer country", Group: "buyer", Required: true,
		Description: "Destination country of the shipment / buyer HQ country"},
	{Key: "consignee_city", Label: "Buyer city", Group: "buyer",
		Description: "City of the importer"},
	{Key: "consignee_address", Label: "Buyer address", Group: "buyer",
		Description: "Full street address of the importer"},
	{Key: "consignee_email", Label: "Buyer email", Group: "buyer", AffectsDimension: DimAccessibility,
		Description: "Direct contact email if available"},
	{Key: "consignee_phone", Label: "Buyer phone", Group: "buyer", AffectsDimension: DimAccessibility,
		Description: "Direct contact phone if available"},
	{Key: "consignee_zip", Label: "Buyer ZIP / postal code", Group: "buyer"},
	{Key: "consignee_state", Label: "Buyer state / region", Group: "buyer"},

	// --- Shipper (origin side) -----------------------------------------
	{Key: "shipper_name", Label: "Supplier / Shipper", Group: "shipper",
		Description: "Exporting company on the shipment"},
	{Key: "shipper_country", Label: "Origin country", Group: "shipper", AffectsDimension: DimFit,
		Description: "Country goods are shipped FROM. Used to gauge fit vs the user's origin."},
	{Key: "shipper_city", Label: "Shipper city", Group: "shipper"},

	// --- Trade flow ----------------------------------------------------
	{Key: "hs_code", Label: "HS code", Group: "trade", AffectsDimension: DimFit,
		Description: "Harmonised System tariff code (any prefix length)"},
	{Key: "product_description", Label: "Product description", Group: "trade", AffectsDimension: DimFit,
		Description: "Free-text product description from the customs filing"},
	{Key: "quantity", Label: "Quantity", Group: "trade", AffectsDimension: DimDealSize,
		Description: "Units / pieces / cartons (whatever the source declares)"},
	{Key: "weight_kg", Label: "Weight (kg)", Group: "trade", AffectsDimension: DimDealSize,
		Description: "Gross or net weight in kilograms"},
	{Key: "total_value_usd", Label: "Total value (USD)", Group: "trade", AffectsDimension: DimDealSize,
		Description: "Declared customs value, normalised to USD if possible"},
	{Key: "shipment_date", Label: "Shipment / arrival date", Group: "trade", AffectsDimension: DimUrgency,
		Description: "Most recent shipment date for this row"},

	// --- Logistics -----------------------------------------------------
	{Key: "unloading_port", Label: "Unloading port", Group: "logistics"},
	{Key: "loading_port", Label: "Loading port", Group: "logistics"},
}

// CanonicalKeys returns just the Key strings — useful when emitting the
// allow-list to the AI mapper prompt.
func CanonicalKeys() []string {
	out := make([]string, len(Canonical))
	for i, f := range Canonical {
		out[i] = f.Key
	}
	return out
}

// CanonicalByKey returns a copy-friendly map indexed by Key. Reused by
// parseRowDynamic and the dimension-floor calculator.
func CanonicalByKey() map[string]FieldDef {
	out := make(map[string]FieldDef, len(Canonical))
	for _, f := range Canonical {
		out[f.Key] = f
	}
	return out
}

// FieldsForDimension returns every canonical field whose absence
// constrains the given scoring dimension.
func FieldsForDimension(dim string) []string {
	var out []string
	for _, f := range Canonical {
		if f.AffectsDimension == dim {
			out = append(out, f.Key)
		}
	}
	return out
}

// RequiredKeys returns the Required=true canonical keys. The mapping UI
// uses this to block submission when any of them is unmapped.
func RequiredKeys() []string {
	var out []string
	for _, f := range Canonical {
		if f.Required {
			out = append(out, f.Key)
		}
	}
	return out
}
