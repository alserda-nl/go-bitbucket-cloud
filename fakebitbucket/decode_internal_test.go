package fakebitbucket

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeSample_PanicsOnABrokenSample(t *testing.T) {
	var into map[string]any

	assert.PanicsWithValue(t, "fakebitbucket: a sample does not decode: unexpected end of JSON input", func() {
		decodeSample([]byte(`{`), &into)
	})
}
