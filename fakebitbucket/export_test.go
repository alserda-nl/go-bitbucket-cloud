package fakebitbucket

import (
	"maps"
	"slices"
)

// RoutePatterns returns the pattern of every route the fake serves, for TestAnswers.
func RoutePatterns(s *Server) []string {
	return slices.Sorted(maps.Keys(s.routes()))
}
