package crawler

type SocialProfile struct {
	Platform   string `json:"platform"`
	URL        string `json:"url"`
	Handle     string `json:"handle,omitempty"`
	SourcePage string `json:"source_page"`
	Confidence string `json:"confidence"`
}

type CrawlResult struct {
	Emails         []string        `json:"emails"`
	SocialProfiles []SocialProfile `json:"social_profiles,omitempty"`
	PagesVisited   int             `json:"pages_visited"`
}
