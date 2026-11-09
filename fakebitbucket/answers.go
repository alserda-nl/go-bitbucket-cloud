package fakebitbucket

import (
	_ "embed"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"strconv"
)

// invalidPagelenSample is Bitbucket's 400 "Invalid pagelen" for a page size a list does not take.
//
//go:embed samples/errors/invalid-pagelen.json
var invalidPagelenSample []byte

// writeSample answers with one of the answers in samples/errors: its status, its content type and
// its body.
func writeSample(w http.ResponseWriter, sample []byte) {
	var answer struct {
		Status      int             `json:"status"`
		ContentType string          `json:"content_type"`
		Body        json.RawMessage `json:"body"`
	}
	decodeSample(sample, &answer)

	w.Header().Set("Content-Type", answer.ContentType)
	w.WriteHeader(answer.Status)

	// A sample's body is null for an answer without one, text for one that is not JSON, or JSON.
	if string(answer.Body) == "null" {
		return
	}

	var text string
	if json.Unmarshal(answer.Body, &text) == nil {
		_, _ = io.WriteString(w, text)
		return
	}

	_, _ = w.Write(answer.Body)
}

// writeJSON answers with status and value as a JSON body.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// errorAnswer is the repository endpoints' error answer: {"type": "error", "error": {"message": ...}}.
type errorAnswer struct {
	// Type is "error".
	Type string `json:"type"`
	// Error holds the error's message.
	Error struct {
		// Message describes the error.
		Message string `json:"message"`
	} `json:"error"`
}

// writeError answers as Bitbucket's repository endpoints answer an error.
func writeError(w http.ResponseWriter, status int, message string) {
	answer := errorAnswer{Type: "error"}
	answer.Error.Message = message

	writeJSON(w, status, answer)
}

// writePage answers the page of items a request's page and pagelen ask for, defaultPageSize items
// a page when it asks none, as Bitbucket's repository endpoints do: ?pagelen=2&page=2 of 5 items
// answers the third and fourth, with "size": 5 and a next link to page 3. A page size outside 1
// to largestPage answers 400 "Invalid pagelen", as Bitbucket answers 101 and 1000.
func writePage(w http.ResponseWriter, r *http.Request, items []any, defaultPageSize, largestPage int) {
	query := r.URL.Query()
	pageSize := defaultPageSize
	if asked := query.Get("pagelen"); asked != "" {
		size, err := strconv.Atoi(asked)
		if err != nil || size < 1 || size > largestPage {
			writeSample(w, invalidPagelenSample)
			return
		}

		pageSize = size
	}

	pageNumber, _ := strconv.Atoi(query.Get("page"))
	pageNumber = max(pageNumber, 1)

	start := min((pageNumber-1)*pageSize, len(items))
	end := min(start+pageSize, len(items))
	answer := map[string]any{
		"page":    pageNumber,
		"pagelen": pageSize,
		"size":    len(items),
		"values":  append([]any{}, items[start:end]...),
	}

	if end < len(items) {
		nextQuery := maps.Clone(query)
		nextQuery.Set("page", strconv.Itoa(pageNumber+1))
		next := *r.URL
		next.Scheme = "http"
		next.Host = r.Host
		next.RawQuery = nextQuery.Encode()
		answer["next"] = next.String()
	}

	writeJSON(w, http.StatusOK, answer)
}
