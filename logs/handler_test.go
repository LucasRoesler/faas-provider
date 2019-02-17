package logs

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func Test_GETRequestParsing(t *testing.T) {
	sinceTime, _ := time.Parse(time.RFC3339, "2019-02-16T09:10:06+00:00")
	testPattern := "^200.*"
	scenarios := []struct {
		name            string
		rawQueryStr     string
		err             string
		expectedRequest Request
	}{
		{
			name:            "empty query creates an empty request",
			rawQueryStr:     "",
			err:             "",
			expectedRequest: Request{},
		},
		{
			name:            "name only query",
			rawQueryStr:     "name=foobar",
			err:             "",
			expectedRequest: Request{Name: "foobar"},
		},
		{
			name:            "name only query",
			rawQueryStr:     "name=foobar",
			err:             "",
			expectedRequest: Request{Name: "foobar"},
		},
		{
			name:            "multiple name values selects the last value",
			rawQueryStr:     "name=foobar&name=theactual name",
			err:             "",
			expectedRequest: Request{Name: "theactual name"},
		},
		{
			name:        "valid request with every parameter",
			rawQueryStr: "name=foobar&since=2019-02-16T09%3A10%3A06%2B00%3A00&instance=abc123&limit=5&follow=true&pattern=%5E200.%2A&invert=true",
			err:         "",
			expectedRequest: Request{
				Name:     "foobar",
				Instance: "abc123",
				Since:    &sinceTime,
				Pattern:  &testPattern,
				Limit:    5,
				Follow:   true,
				Invert:   true,
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			req.URL.RawQuery = s.rawQueryStr
			logRequest, err := parseRequest(req)
			equalError(t, s.err, err)

			if logRequest.String() != s.expectedRequest.String() {
				t.Errorf("expected log request: %s, got: %s", s.expectedRequest, logRequest)
			}
		})
	}
}

func equalError(t *testing.T, expected string, actual error) {
	if expected == "" && actual == nil {
		return
	}

	if expected == "" && actual != nil {
		t.Errorf("unexpected error: %s", actual.Error())
		return
	}

	if actual.Error() != expected {
		t.Errorf("expected error: %s got: %s", expected, actual.Error())
	}
}
