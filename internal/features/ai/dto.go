package ai

// GeneratePostRequest adalah input untuk AI Blog Generator.
type GeneratePostRequest struct {
	Topic            string `json:"topic"`
	Prompt           string `json:"prompt"`
	Category         string `json:"category"`
	IncludeWebSearch bool   `json:"include_web_search"`
}

// GeneratedPost adalah struktur artikel yang dikembalikan dari model AI.
type GeneratedPost struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Excerpt  string `json:"excerpt"`
	Content  string `json:"content"`
}

// GeneratePostResponse adalah balasan yang dikirim ke frontend.
type GeneratePostResponse struct {
	Title         string   `json:"title"`
	Category      string   `json:"category"`
	Excerpt       string   `json:"excerpt"`
	Content       string   `json:"content"`
	SearchSources []string `json:"search_sources"`
}

type SearchResultItem struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

type SearchEngineResponse struct {
	Results []SearchResultItem `json:"results"`
}