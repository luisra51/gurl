package crawler

type SocialProfile struct {
	Platform   string `json:"platform"`
	URL        string `json:"url"`
	Handle     string `json:"handle,omitempty"`
	SourcePage string `json:"source_page"`
	Confidence string `json:"confidence"`
}

// Phone represents a phone number discovered during crawling, normalized
// to E.164 when possible.
type Phone struct {
	Number     string `json:"number"`          // E.164 (e.g. "+56229386600") when valid
	Raw        string `json:"raw,omitempty"`   // original string as seen on the page
	Source     string `json:"source"`          // "tel_link" | "jsonld" | "text"
	SourcePage string `json:"source_page"`
	Confidence string `json:"confidence"` // "high" | "medium" | "low"
}

// PostalAddress mirrors the schema.org PostalAddress fields we care about.
type PostalAddress struct {
	StreetAddress   string `json:"street_address,omitempty"`
	AddressLocality string `json:"address_locality,omitempty"`
	AddressRegion   string `json:"address_region,omitempty"`
	PostalCode      string `json:"postal_code,omitempty"`
	AddressCountry  string `json:"address_country,omitempty"`
}

// Organization captures structured data extracted from JSON-LD nodes whose
// @type is Organization, LocalBusiness, Corporation, or similar subclasses.
type Organization struct {
	Name        string         `json:"name,omitempty"`
	LegalName   string         `json:"legal_name,omitempty"`
	Description string         `json:"description,omitempty"`
	URL         string         `json:"url,omitempty"`
	Email       string         `json:"email,omitempty"`
	Phone       string         `json:"phone,omitempty"`
	Address     *PostalAddress `json:"address,omitempty"`
	SameAs      []string       `json:"same_as,omitempty"`
	SourcePage  string         `json:"source_page"`
}

type CrawlResult struct {
	Emails         []string        `json:"emails"`
	SocialProfiles []SocialProfile `json:"social_profiles,omitempty"`
	Phones         []Phone         `json:"phones,omitempty"`
	Organizations  []Organization  `json:"organizations,omitempty"`
	PagesVisited   int             `json:"pages_visited"`
}
