package search

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Document represents an indexable content item.
type Document struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"` // e.g. "articles", "pages"
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	Tags     []string       `json:"tags,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SearchResult represents a scored search match.
type SearchResult struct {
	Document   Document `json:"document"`
	Score      float64  `json:"score"`
	Highlights []string `json:"highlights,omitempty"`
}

var wordRegex = regexp.MustCompile(`[\p{L}\p{N}]+`)

var stopWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "and": {}, "or": {}, "in": {}, "on": {}, "at": {},
	"of": {}, "to": {}, "for": {}, "with": {}, "by": {}, "from": {}, "is": {}, "are": {},
	"was": {}, "were": {}, "be": {}, "this": {}, "that": {}, "it": {},
}

// Engine implements an in-memory inverted full-text search engine.
type Engine struct {
	mu        sync.RWMutex
	docs      map[string]Document
	postings  map[string]map[string]float64 // token -> docID -> TF weight
	docCounts map[string]int                // docID -> total word count
}

// NewEngine creates a new search Engine.
func NewEngine() *Engine {
	return &Engine{
		docs:      make(map[string]Document),
		postings:  make(map[string]map[string]float64),
		docCounts: make(map[string]int),
	}
}

func tokenize(text string) []string {
	words := wordRegex.FindAllString(strings.ToLower(text), -1)
	res := make([]string, 0, len(words))
	for _, w := range words {
		if _, isStop := stopWords[w]; !isStop && len(w) > 1 {
			res = append(res, w)
		}
	}
	return res
}

// Index adds or updates a document in the search index.
func (e *Engine) Index(doc Document) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// If already present, remove old postings first
	e.deleteLocked(doc.ID)

	e.docs[doc.ID] = doc

	titleTokens := tokenize(doc.Title)
	bodyTokens := tokenize(doc.Body)
	tagTokens := make([]string, 0)
	for _, tag := range doc.Tags {
		tagTokens = append(tagTokens, tokenize(tag)...)
	}

	totalWords := len(titleTokens) + len(bodyTokens) + len(tagTokens)
	if totalWords == 0 {
		return
	}
	e.docCounts[doc.ID] = totalWords

	// Title tokens weighted 3.0x, tags 2.0x, body 1.0x
	tokenWeights := make(map[string]float64)
	for _, t := range titleTokens {
		tokenWeights[t] += 3.0
	}
	for _, t := range tagTokens {
		tokenWeights[t] += 2.0
	}
	for _, t := range bodyTokens {
		tokenWeights[t] += 1.0
	}

	for token, weight := range tokenWeights {
		if _, exists := e.postings[token]; !exists {
			e.postings[token] = make(map[string]float64)
		}
		// Normalized term frequency
		e.postings[token][doc.ID] = weight / float64(totalWords)
	}
}

// Delete removes a document from the search index.
func (e *Engine) Delete(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.deleteLocked(id)
}

func (e *Engine) deleteLocked(id string) {
	delete(e.docs, id)
	delete(e.docCounts, id)

	for token, docMap := range e.postings {
		delete(docMap, id)
		if len(docMap) == 0 {
			delete(e.postings, token)
		}
	}
}

// Search evaluates a search query against indexed documents using TF-IDF scoring.
func (e *Engine) Search(query string, typeFilter string, limit int) []SearchResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return []SearchResult{}
	}

	totalDocs := float64(len(e.docs))
	if totalDocs == 0 {
		return []SearchResult{}
	}

	scores := make(map[string]float64)

	for _, token := range queryTokens {
		// Exact match
		if docMap, exists := e.postings[token]; exists {
			docFreq := float64(len(docMap))
			idf := math.Log(1.0 + (totalDocs / docFreq))

			for docID, tf := range docMap {
				scores[docID] += tf * idf
			}
		}

		// Prefix / wildcard matching for partial query terms
		for indexToken, docMap := range e.postings {
			if indexToken != token && strings.HasPrefix(indexToken, token) {
				docFreq := float64(len(docMap))
				idf := math.Log(1.0 + (totalDocs / docFreq)) * 0.5 // partial match discount

				for docID, tf := range docMap {
					scores[docID] += tf * idf
				}
			}
		}
	}

	results := make([]SearchResult, 0, len(scores))
	for docID, score := range scores {
		doc := e.docs[docID]
		if typeFilter != "" && doc.Type != typeFilter {
			continue
		}

		// Build highlight snippet
		snippet := buildSnippet(doc.Body, queryTokens)

		results = append(results, SearchResult{
			Document:   doc,
			Score:      math.Round(score*1000) / 1000,
			Highlights: []string{snippet},
		})
	}

	// Sort results descending by score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

func buildSnippet(body string, tokens []string) string {
	if body == "" {
		return ""
	}
	lower := strings.ToLower(body)
	for _, tok := range tokens {
		idx := strings.Index(lower, tok)
		if idx >= 0 {
			start := idx - 30
			if start < 0 {
				start = 0
			}
			end := idx + len(tok) + 60
			if end > len(body) {
				end = len(body)
			}
			snip := strings.TrimSpace(body[start:end])
			if start > 0 {
				snip = "..." + snip
			}
			if end < len(body) {
				snip = snip + "..."
			}
			return snip
		}
	}

	if len(body) > 100 {
		return body[:100] + "..."
	}
	return body
}

// Clear flushes all index structures.
func (e *Engine) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.docs = make(map[string]Document)
	e.postings = make(map[string]map[string]float64)
	e.docCounts = make(map[string]int)
}

