package models_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// readSample reads one of the fake's samples: real answers, with names and IDs replaced.
func readSample(t *testing.T, name string) []byte {
	t.Helper()

	sample, err := os.ReadFile(fmt.Sprintf("../fakebitbucket/samples/%s", name))
	require.NoError(t, err)

	return sample
}

// missingFields encodes decoded again and returns every field of sample it lost, or holds as
// another JSON type. Fields only decoded has are fine: a short form leaves fields zero.
func missingFields(t *testing.T, sample []byte, decoded any) []string {
	t.Helper()

	encoded, err := json.Marshal(decoded)
	require.NoError(t, err)

	var expected, actual any
	require.NoError(t, json.Unmarshal(sample, &expected))
	require.NoError(t, json.Unmarshal(encoded, &actual))

	return compareFields("", expected, actual)
}

func compareFields(path string, expected, actual any) []string {
	switch expectedValue := expected.(type) {
	case map[string]any:
		actualValue, ok := actual.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: want an object, got %T", path, actual)}
		}

		var missing []string
		for key, item := range expectedValue {
			if _, found := actualValue[key]; !found {
				missing = append(missing, fmt.Sprintf("%s.%s: missing", path, key))
				continue
			}

			missing = append(missing, compareFields(fmt.Sprintf("%s.%s", path, key), item, actualValue[key])...)
		}

		return missing
	case nil:
		return nil
	default:
		if fmt.Sprintf("%T", expected) != fmt.Sprintf("%T", actual) {
			return []string{fmt.Sprintf("%s: want %T, got %T", path, expected, actual)}
		}

		return nil
	}
}
