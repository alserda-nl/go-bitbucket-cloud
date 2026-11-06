package fakebitbucket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteSample_WritesTheBodyInTheFormTheSampleHolds(t *testing.T) {
	cases := []struct {
		name                string
		sample              string
		expectedStatus      int
		expectedContentType string
		expectedBody        string
	}{
		{
			name:                "an answer in JSON",
			sample:              "errors/invalid-pagelen.json",
			expectedStatus:      http.StatusBadRequest,
			expectedContentType: "application/json; charset=utf-8",
			expectedBody:        `{"error": {"message": "Invalid pagelen"}, "type": "error"}`,
		},
		{
			name:                "an answer in text",
			sample:              "errors/text-answer.json",
			expectedStatus:      http.StatusBadRequest,
			expectedContentType: "text/plain",
			expectedBody:        "Bad request",
		},
		{
			name:                "an answer without a body",
			sample:              "errors/file-not-found.json",
			expectedStatus:      http.StatusNotFound,
			expectedContentType: "application/json",
			expectedBody:        "",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			sample, err := os.ReadFile(fmt.Sprintf("samples/%s", testCase.sample))
			require.NoError(t, err)
			recorder := httptest.NewRecorder()

			writeSample(recorder, sample)

			assert.Equal(t, testCase.expectedStatus, recorder.Code, "writeSample(%s)", testCase.sample)
			assert.Equal(t, testCase.expectedContentType, recorder.Header().Get("Content-Type"), "writeSample(%s)", testCase.sample)
			if testCase.expectedBody == "" || testCase.expectedBody[0] != '{' {
				assert.Equal(t, testCase.expectedBody, recorder.Body.String(), "writeSample(%s)", testCase.sample)
			} else {
				assert.JSONEq(t, testCase.expectedBody, recorder.Body.String(), "writeSample(%s)", testCase.sample)
			}
		})
	}
}
