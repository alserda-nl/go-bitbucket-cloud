package fakebitbucket

import (
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

// lastID numbers every object a builder makes, so no two builder calls in a test binary return
// the same UUID, slug or key.
//
//nolint:gochecknoglobals // one counter for every builder, so no two calls share an ID
var lastID atomic.Int64

// nextID returns a number no builder call returned before, from 1,000,001 up, so a builder's
// UUID never equals one of the placeholder UUIDs in the samples, which count from 1.
func nextID() int64 {
	return 1_000_000 + lastID.Add(1)
}

// uuidFor returns a UUID in braces, as Bitbucket writes one, made from a builder's number.
func uuidFor(id int64) models.UUID {
	return fmt.Sprintf("{00000000-0000-4000-8000-%012d}", id)
}

// decodeSample decodes one of the embedded samples; a sample that does not decode is a broken build.
func decodeSample(sample []byte, into any) {
	if err := json.Unmarshal(sample, into); err != nil {
		panic(fmt.Sprintf("fakebitbucket: a sample does not decode: %v", err))
	}
}
