package discovery

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ShipmentRecord is a single row from the Tendata/customs Excel export.
type ShipmentRecord struct {
	ArrivalDate        string
	ConsigneeName      string
	ConsigneeStdName   string
	ConsigneeAddress   string
	ConsigneeState     string
	ConsigneeCity      string
	ConsigneeZip       string
	ConsigneeCountry   string
	ConsigneePhone     string
	ConsigneeEmail     string // populated by parseRowDynamic when consignee_email is mapped
	ShipperName        string
	ShipperStdName     string
	ShipperCity        string
	ShipperCountry     string
	HSCode             string
	ProductDescription string
	Quantity           float64
	GrossWeightKG      float64
	TotalPrice         float64
	UnloadingPort      string
	OriginCountry      string
	// FieldPresence is set only by ParseExcelWithMapping. nil for the
	// legacy ParseTendataExcelRaw path; downstream code should treat nil
	// as "presence unknown — fall back to looking at the value itself".
	FieldPresence map[string]bool
}

// SupplierInfo holds a supplier (shipper) with their country.
type SupplierInfo struct {
	Name    string `json:"name"`
	Country string `json:"country"`
	City    string `json:"city"`
}

// ShipmentAggregation holds aggregated customs data for a single consignee (importer).
type ShipmentAggregation struct {
	CompanyName      string         `json:"company_name"`
	Address          string         `json:"address"`
	City             string         `json:"city"`
	State            string         `json:"state"`
	Country          string         `json:"country"`
	ZipCode          string         `json:"zip_code"`
	Phone            string         `json:"phone"`
	Email            string         `json:"email"`
	TransactionCount int            `json:"transaction_count"`
	TotalWeightKG    float64        `json:"total_weight_kg"`
	TotalValueUSD    float64        `json:"total_value_usd"`
	LastShipmentDate string         `json:"last_shipment_date"`
	HSCodes          []string       `json:"hs_codes"`
	Products         []string       `json:"products"`
	Suppliers        []SupplierInfo `json:"suppliers"`
	BuysFromHome     bool           `json:"buys_from_home"`
	TrustTier        string         `json:"trust_tier"` // "confirmed_buyer" or "confirmed_importer"
	// FieldPresence is the union of all rows' canonical-field presence
	// flags for this importer. If ANY row had `consignee_email` mapped
	// and non-empty, FieldPresence["consignee_email"] is true.
	FieldPresence map[string]bool `json:"field_presence,omitempty"`
}

// ExcelImportResult holds the parsed and aggregated output.
type ExcelImportResult struct {
	Importers    []ShipmentAggregation `json:"importers"`
	TotalRows    int                   `json:"total_rows"`
	Uniquebuyers int                   `json:"unique_buyers"`
	SkippedRows  int                   `json:"skipped_rows"`
}

// ParseTendataExcel reads a Tendata/customs Excel export from an io.Reader,
// aggregates by consignee, and tags trust tiers based on homeCountry.
func ParseTendataExcel(reader io.Reader, homeCountry string) (*ExcelImportResult, error) {
	records, totalRows, skipped, err := ParseTendataExcelRaw(reader)
	if err != nil {
		return nil, err
	}

	homeNorm := normalizeCountry(homeCountry)
	importers := aggregateByConsignee(records, homeNorm)

	return &ExcelImportResult{
		Importers:    importers,
		TotalRows:    totalRows,
		Uniquebuyers: len(importers),
		SkippedRows:  skipped,
	}, nil
}

// ParseTendataExcelRaw returns the raw parsed shipment records without aggregation.
// Used by the new Excel-only discovery pipeline which splits records by destination
// before aggregating per destination-market.
func ParseTendataExcelRaw(reader io.Reader) (records []ShipmentRecord, totalRows, skipped int, err error) {
	f, err := excelize.OpenReader(reader)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("excel: open: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, 0, 0, fmt.Errorf("excel: no sheets found")
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("excel: read rows: %w", err)
	}
	if len(rows) < 2 {
		return nil, 0, 0, fmt.Errorf("excel: file has no data rows (only %d rows)", len(rows))
	}

	colIdx := buildColumnIndex(rows[0])
	totalRows = len(rows) - 1

	for i := 1; i < len(rows); i++ {
		rec := parseRow(rows[i], colIdx)
		if rec.ConsigneeStdName == "" && rec.ConsigneeName == "" {
			skipped++
			continue
		}
		records = append(records, rec)
	}

	return records, totalRows, skipped, nil
}

// AggregateRecords runs the per-consignee aggregation on a subset of records
// (used by the new Excel-only pipeline, one call per destination-split group).
func AggregateRecords(records []ShipmentRecord, homeCountry string) []ShipmentAggregation {
	return aggregateByConsignee(records, normalizeCountry(homeCountry))
}

// ParseExcelWithMapping is the dynamic parser. It accepts a user-confirmed
// {header → canonical_key} mapping and reads each row by canonical key.
// Multiple headers can map to the same canonical key (e.g. "Phone 1",
// "Phone 2" both → consignee_phone) — the first non-empty value wins.
//
// Each parsed record carries a FieldPresence map that flags which canonical
// keys actually had a non-empty value for THAT row, so downstream scoring
// can cap dimensions whose inputs were absent.
func ParseExcelWithMapping(reader io.Reader, mapping map[string]string) (records []ShipmentRecord, totalRows, skipped int, err error) {
	f, openErr := excelize.OpenReader(reader)
	if openErr != nil {
		return nil, 0, 0, fmt.Errorf("excel: open: %w", openErr)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, 0, 0, fmt.Errorf("excel: no sheets found")
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("excel: read rows: %w", err)
	}
	if len(rows) < 2 {
		return nil, 0, 0, fmt.Errorf("excel: file has no data rows (only %d rows)", len(rows))
	}

	headers := rows[0]

	// Build canonical → ordered list of column indices (one canonical key
	// can have multiple source headers).
	canonToCols := map[string][]int{}
	for colIdx, h := range headers {
		canon, ok := mapping[h]
		if !ok || canon == "" {
			continue
		}
		canonToCols[canon] = append(canonToCols[canon], colIdx)
	}

	totalRows = len(rows) - 1
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		rec, presence := parseRowDynamic(row, canonToCols)
		if rec.ConsigneeName == "" && rec.ConsigneeStdName == "" {
			skipped++
			continue
		}
		rec.FieldPresence = presence
		records = append(records, rec)
	}
	return records, totalRows, skipped, nil
}

// parseRowDynamic is the per-row reader for the dynamic mapping path.
// It pulls the first non-empty value for each canonical key from any of
// the columns the user mapped, and returns the populated ShipmentRecord
// plus a presence map flagging which canonical keys had data.
func parseRowDynamic(row []string, canonToCols map[string][]int) (ShipmentRecord, map[string]bool) {
	get := func(canon string) string {
		for _, c := range canonToCols[canon] {
			if c < len(row) {
				if v := strings.TrimSpace(row[c]); v != "" {
					return v
				}
			}
		}
		return ""
	}

	rec := ShipmentRecord{
		ArrivalDate:        get("shipment_date"),
		ConsigneeName:      get("consignee_name"),
		ConsigneeStdName:   get("consignee_name"),
		ConsigneeAddress:   get("consignee_address"),
		ConsigneeState:     get("consignee_state"),
		ConsigneeCity:      get("consignee_city"),
		ConsigneeZip:       get("consignee_zip"),
		ConsigneeCountry:   get("consignee_country"),
		ConsigneePhone:     get("consignee_phone"),
		ConsigneeEmail:     get("consignee_email"),
		ShipperName:        get("shipper_name"),
		ShipperStdName:     get("shipper_name"),
		ShipperCity:        get("shipper_city"),
		ShipperCountry:     get("shipper_country"),
		HSCode:             get("hs_code"),
		ProductDescription: get("product_description"),
		Quantity:           parseFloat(get("quantity")),
		GrossWeightKG:      parseFloat(get("weight_kg")),
		TotalPrice:         parseFloat(get("total_value_usd")),
		UnloadingPort:      get("unloading_port"),
		OriginCountry:      get("shipper_country"),
	}

	// Presence reflects only canonical keys that produced a non-empty
	// value for THIS row — even if mapping included the column.
	presence := make(map[string]bool, len(canonToCols))
	for canon := range canonToCols {
		presence[canon] = get(canon) != ""
	}
	// Special-case: consignee_email isn't in ShipmentRecord today but the
	// importer wants to know if it was present so scoring can later cap
	// accessibility correctly. We track it in presence even though the
	// value isn't yet stored on the record.
	if get("consignee_email") != "" {
		presence["consignee_email"] = true
	}
	return rec, presence
}

// columnIndex maps known header names to their column positions.
type columnIndex struct {
	cols map[string]int
}

func buildColumnIndex(header []string) columnIndex {
	idx := columnIndex{cols: make(map[string]int)}
	for i, h := range header {
		normalized := strings.TrimSpace(strings.ToLower(h))
		idx.cols[normalized] = i
	}
	return idx
}

func (ci columnIndex) get(row []string, names ...string) string {
	for _, name := range names {
		if i, ok := ci.cols[strings.ToLower(name)]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
	}
	return ""
}

func parseRow(row []string, ci columnIndex) ShipmentRecord {
	return ShipmentRecord{
		ArrivalDate:        ci.get(row, "Arrival Date"),
		ConsigneeName:      ci.get(row, "Consignee Name"),
		ConsigneeStdName:   ci.get(row, "Consignee Std Name"),
		ConsigneeAddress:   ci.get(row, "Consignee Add."),
		ConsigneeState:     ci.get(row, "Consignee State"),
		ConsigneeCity:      ci.get(row, "Consignee City"),
		ConsigneeZip:       ci.get(row, "Zip Code"),
		ConsigneeCountry:   ci.get(row, "Consignee Country(EN)", "Consignee Country", "Destination Country", "Country"),
		ConsigneePhone:     ci.get(row, "Comm No."),
		ShipperName:        ci.get(row, "Shipper Std Name", "Shipper Name"),
		ShipperStdName:     ci.get(row, "Shipper Std Name"),
		ShipperCity:        ci.get(row, "Shipper City"),
		ShipperCountry:     coalesce(ci.get(row, "Shipper Country(EN)"), ci.get(row, "Origin Ctry")),
		HSCode:             ci.get(row, "HS Code"),
		ProductDescription: ci.get(row, "Product Description", "Product(EN)"),
		Quantity:           parseFloat(ci.get(row, "Quantity")),
		GrossWeightKG:      parseFloat(ci.get(row, "Container Product Gross Weight")),
		TotalPrice:         parseFloat(ci.get(row, "Total Price")),
		UnloadingPort:      ci.get(row, "Unloading Port"),
		OriginCountry:      ci.get(row, "Origin Ctry"),
	}
}

func aggregateByConsignee(records []ShipmentRecord, homeCountry string) []ShipmentAggregation {
	groups := make(map[string]*ShipmentAggregation)
	order := make([]string, 0) // preserve insertion order

	for _, rec := range records {
		key := strings.ToUpper(rec.ConsigneeStdName)
		if key == "" {
			key = strings.ToUpper(rec.ConsigneeName)
		}
		if key == "" {
			continue
		}

		agg, exists := groups[key]
		if !exists {
			agg = &ShipmentAggregation{
				CompanyName:   coalesce(rec.ConsigneeStdName, rec.ConsigneeName),
				Address:       rec.ConsigneeAddress,
				City:          rec.ConsigneeCity,
				State:         rec.ConsigneeState,
				Country:       rec.ConsigneeCountry,
				ZipCode:       rec.ConsigneeZip,
				Phone:         rec.ConsigneePhone,
				Email:         rec.ConsigneeEmail,
				FieldPresence: map[string]bool{},
			}
			groups[key] = agg
			order = append(order, key)
		}
		// Merge per-row presence flags into the aggregate. Once any row
		// for this importer had a canonical field, the aggregate carries
		// it as present.
		for k, v := range rec.FieldPresence {
			if v {
				agg.FieldPresence[k] = true
			}
		}
		// Backfill email when a later row supplies one.
		if rec.ConsigneeEmail != "" && agg.Email == "" {
			agg.Email = rec.ConsigneeEmail
		}

		agg.TransactionCount++
		agg.TotalWeightKG += rec.GrossWeightKG
		agg.TotalValueUSD += rec.TotalPrice

		// Track last shipment date
		if rec.ArrivalDate > agg.LastShipmentDate {
			agg.LastShipmentDate = rec.ArrivalDate
		}

		// Collect unique HS codes
		if rec.HSCode != "" && !containsStr(agg.HSCodes, rec.HSCode) {
			agg.HSCodes = append(agg.HSCodes, rec.HSCode)
		}

		// Collect unique products
		prodUpper := strings.ToUpper(rec.ProductDescription)
		if prodUpper != "" && !containsStr(agg.Products, prodUpper) {
			agg.Products = append(agg.Products, rec.ProductDescription)
		}

		// Collect unique suppliers
		if rec.ShipperName != "" {
			supplierKey := strings.ToUpper(rec.ShipperName)
			found := false
			for _, s := range agg.Suppliers {
				if strings.ToUpper(s.Name) == supplierKey {
					found = true
					break
				}
			}
			if !found {
				agg.Suppliers = append(agg.Suppliers, SupplierInfo{
					Name:    coalesce(rec.ShipperStdName, rec.ShipperName),
					Country: rec.ShipperCountry,
					City:    rec.ShipperCity,
				})
			}

			// Check if supplier is from user's home country
			if normalizeCountry(rec.ShipperCountry) == homeCountry {
				agg.BuysFromHome = true
			}
		}
	}

	// Build result slice in original order, sorted by transaction count
	result := make([]ShipmentAggregation, 0, len(groups))
	for _, key := range order {
		agg := groups[key]
		if agg.BuysFromHome {
			agg.TrustTier = "confirmed_buyer"
		} else {
			agg.TrustTier = "confirmed_importer"
		}
		// Clean up date format
		if len(agg.LastShipmentDate) >= 10 {
			agg.LastShipmentDate = agg.LastShipmentDate[:10]
		}
		result = append(result, *agg)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].TransactionCount > result[j].TransactionCount
	})

	return result
}

// shipmentDataJSON builds the JSON metadata stored in the shipment_data column.
func shipmentDataJSON(agg ShipmentAggregation) json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"transaction_count":  agg.TransactionCount,
		"total_weight_kg":    agg.TotalWeightKG,
		"total_value_usd":    agg.TotalValueUSD,
		"last_shipment_date": agg.LastShipmentDate,
		"hs_codes":           agg.HSCodes,
		"products":           agg.Products,
		"suppliers":          agg.Suppliers,
		"buys_from_home":     agg.BuysFromHome,
		"trust_tier":         agg.TrustTier,
	})
	return data
}

// normalizeCountry normalizes country names/codes for comparison.
func normalizeCountry(s string) string {
	s = strings.TrimSpace(strings.ToUpper(s))
	// Handle common variants
	countryMap := map[string]string{
		"TR": "TURKEY", "TURKIYE": "TURKEY", "TÜRKIYE": "TURKEY",
		"US": "UNITED STATES", "USA": "UNITED STATES", "UNITED STATES OF AMERICA": "UNITED STATES",
		"DE": "GERMANY", "GB": "UNITED KINGDOM", "UK": "UNITED KINGDOM",
		"FR": "FRANCE", "IT": "ITALY", "ES": "SPAIN", "NL": "NETHERLANDS",
		"CN": "CHINA", "JP": "JAPAN", "KR": "SOUTH KOREA",
		"SA": "SAUDI ARABIA", "AE": "UNITED ARAB EMIRATES",
		"BR": "BRAZIL", "IN": "INDIA", "RU": "RUSSIA",
	}
	if mapped, ok := countryMap[s]; ok {
		return mapped
	}
	return s
}

func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "None" {
		return 0
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// importedToBusiness converts aggregated importers to Business records for the pipeline.
func importedToBusiness(importers []ShipmentAggregation, countryCode string) []importedBusinessRecord {
	records := make([]importedBusinessRecord, 0, len(importers))
	for _, imp := range importers {
		ds := "tendata_import"
		bt := imp.TrustTier

		records = append(records, importedBusinessRecord{
			Name:         imp.CompanyName,
			Address:      imp.Address,
			City:         imp.City,
			State:        imp.State,
			CountryCode:  countryCode,
			ZipCode:      imp.ZipCode,
			Phone:        imp.Phone,
			DataSource:   ds,
			BusinessType: bt,
			ShipmentData: shipmentDataJSON(imp),
		})
	}
	return records
}

type importedBusinessRecord struct {
	Name         string
	Address      string
	City         string
	State        string
	CountryCode  string
	ZipCode      string
	Phone        string
	DataSource   string
	BusinessType string
	ShipmentData json.RawMessage
}

// shipmentContextForAI builds a text summary of shipment data for AI prompts.
func shipmentContextForAI(shipmentData json.RawMessage) string {
	if shipmentData == nil {
		return ""
	}

	var sd struct {
		TransactionCount int            `json:"transaction_count"`
		TotalWeightKG    float64        `json:"total_weight_kg"`
		TotalValueUSD    float64        `json:"total_value_usd"`
		LastShipmentDate string         `json:"last_shipment_date"`
		Products         []string       `json:"products"`
		Suppliers        []SupplierInfo `json:"suppliers"`
		BuysFromHome     bool           `json:"buys_from_home"`
		TrustTier        string         `json:"trust_tier"`
	}

	if err := json.Unmarshal(shipmentData, &sd); err != nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Trust: %s", sd.TrustTier))
	if sd.BuysFromHome {
		sb.WriteString(" (already imports from your country)")
	} else {
		sb.WriteString(" (imports from other countries)")
	}

	sb.WriteString(fmt.Sprintf(" | %d shipments, %.0f kg total", sd.TransactionCount, sd.TotalWeightKG))

	if sd.TotalValueUSD > 0 {
		sb.WriteString(fmt.Sprintf(", $%.0f declared value", sd.TotalValueUSD))
	}

	if sd.LastShipmentDate != "" {
		sb.WriteString(fmt.Sprintf(" | Last: %s", sd.LastShipmentDate))
	}

	if len(sd.Products) > 0 {
		sb.WriteString(fmt.Sprintf(" | Products: %s", strings.Join(sd.Products, ", ")))
	}

	if len(sd.Suppliers) > 0 {
		names := make([]string, 0, len(sd.Suppliers))
		for _, s := range sd.Suppliers {
			entry := s.Name
			if s.Country != "" {
				entry += " (" + s.Country + ")"
			}
			names = append(names, entry)
		}
		sb.WriteString(fmt.Sprintf(" | Current suppliers: %s", strings.Join(names, ", ")))
	}

	return sb.String()
}

