package api

// Article is an article summary returned by the journal API.
type Article struct {
	ID        int      `json:"id"`
	Title     string   `json:"title"`
	WordCount int      `json:"word_count"`
	CreatedAt string   `json:"created_at"`
	Journal   *Journal `json:"journal"`
}

// Journal identifies an article journal.
type Journal struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// ArticleContents is the full Markdown response for an article.
type ArticleContents struct {
	Contents string `json:"contents"`
}

type articleContentsRequest struct {
	Contents string `json:"contents"`
}

type articleJournalRequest struct {
	Journal string `json:"journal"`
}

// SavedTracksResponse is the response from the saved albums endpoint.
type SavedTracksResponse struct {
	Data []struct {
		Line string `json:"line"`
	} `json:"data"`
}
