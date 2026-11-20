package ai

// GeneratePostRequest is the AI Blog Generator input.
// Category is an optional hint; IncludeWebSearch enables the search engine step.
type GeneratePostRequest struct {
	Topic            string `json:"topic"`
	Prompt           string `json:"prompt"`
	Category         string `json:"category"`
	IncludeWebSearch bool   `json:"include_web_search"`
}

// GeneratedPost is the structured article returned by the language model.
type GeneratedPost struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Excerpt  string `json:"excerpt"`
	Content  string `json:"content"`
}

// GeneratePostResponse is returned to the frontend. SearchSources is always an
// array (empty when web search was not used).
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
