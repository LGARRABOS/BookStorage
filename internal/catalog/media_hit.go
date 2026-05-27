package catalog

// CatalogMediaHit is a normalized catalog browse/search result from any upstream source.
type CatalogMediaHit struct {
	Source      string   `json:"source"`
	ExternalID  string   `json:"external_id"`
	Title       string   `json:"title"`
	ReadingType string   `json:"reading_type"`
	ImageURL    string   `json:"image_url,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	IsAdult     bool     `json:"is_adult"`
}
