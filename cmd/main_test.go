// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"encoding/base64"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus-community/json_exporter/config"
	pconfig "github.com/prometheus/common/config"
)

func TestFailIfSelfSignedCA(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer target.Close()

	req := httptest.NewRequest("GET", "http://example.com/foo"+"?target="+target.URL, nil)
	recorder := httptest.NewRecorder()
	probeHandler(recorder, req, log.NewNopLogger(), config.Config{})

	resp := recorder.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Fail if (not strict) selfsigned CA test fails unexpectedly, got %s", body)
	}
}

func TestSucceedIfSelfSignedCA(t *testing.T) {
	c := config.Config{}
	c.HTTPClientConfig.TLSConfig.InsecureSkipVerify = true
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))
	defer target.Close()

	req := httptest.NewRequest("GET", "http://example.com/foo"+"?target="+target.URL, nil)
	recorder := httptest.NewRecorder()
	probeHandler(recorder, req, log.NewNopLogger(), c)

	resp := recorder.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Succeed if (not strict) selfsigned CA test fails unexpectedly, got %s", body)
	}
}

func TestFailIfTargetMissing(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	recorder := httptest.NewRecorder()
	probeHandler(recorder, req, log.NewNopLogger(), config.Config{})

	resp := recorder.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Fail if 'target' query parameter missing test fails unexpectedly, got %s", body)
	}
}

func TestDefaultAcceptHeader(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "application/json"
		if got := r.Header.Get("Accept"); got != expected {
			t.Errorf("Default 'Accept' header mismatch, got %s, expected: %s", got, expected)
			w.WriteHeader(http.StatusNotAcceptable)
		}
	}))
	defer target.Close()

	req := httptest.NewRequest("GET", "http://example.com/foo"+"?target="+target.URL, nil)
	recorder := httptest.NewRecorder()
	probeHandler(recorder, req, log.NewNopLogger(), config.Config{})

	resp := recorder.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Default 'Accept: application/json' header test fails unexpectedly, got %s", body)
	}
}

func TestCorrectResponse(t *testing.T) {
	tests := []struct {
		ConfigFile    string
		ServeFile     string
		ResponseFile  string
		ShouldSucceed bool
	}{
		{"../test/config/good.yml", "/serve/good.json", "../test/response/good.txt", true},
		{"../test/config/good.yml", "/serve/repeat-metric.json", "../test/response/good.txt", false},
	}

	target := httptest.NewServer(http.FileServer(http.Dir("../test")))
	defer target.Close()

	for i, test := range tests {
		c, err := config.LoadConfig(test.ConfigFile)
		if err != nil {
			t.Fatalf("Failed to load config file %s", test.ConfigFile)
		}

		req := httptest.NewRequest("GET", "http://example.com/foo"+"?target="+target.URL+test.ServeFile, nil)
		recorder := httptest.NewRecorder()
		probeHandler(recorder, req, log.NewNopLogger(), c)

		resp := recorder.Result()
		body, _ := ioutil.ReadAll(resp.Body)

		expected, _ := ioutil.ReadFile(test.ResponseFile)

		if test.ShouldSucceed && string(body) != string(expected) {
			t.Fatalf("Correct response validation test %d fails unexpectedly.\nGOT:\n%s\nEXPECTED:\n%s", i, body, expected)
		}
	}
}

func TestBasicAuth(t *testing.T) {
	username := "myUser"
	password := "mySecretPassword"
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != expected {
			t.Errorf("BasicAuth mismatch, got: %s, expected: %s", got, expected)
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer target.Close()

	req := httptest.NewRequest("GET", "http://example.com/foo"+"?target="+target.URL, nil)
	recorder := httptest.NewRecorder()
	c := config.Config{}
	auth := &pconfig.BasicAuth{
		Username: username,
		Password: pconfig.Secret(password),
	}

	c.HTTPClientConfig.BasicAuth = auth
	probeHandler(recorder, req, log.NewNopLogger(), c)

	resp := recorder.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("BasicAuth test fails unexpectedly. Got: %s", body)
	}
}

func TestBearerToken(t *testing.T) {
	token := "mySecretToken"
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "Bearer " + token
		if got := r.Header.Get("Authorization"); got != expected {
			t.Errorf("BearerToken mismatch, got: %s, expected: %s", got, expected)
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer target.Close()

	req := httptest.NewRequest("GET", "http://example.com/foo"+"?target="+target.URL, nil)
	recorder := httptest.NewRecorder()
	c := config.Config{}

	c.HTTPClientConfig.BearerToken = pconfig.Secret(token)
	probeHandler(recorder, req, log.NewNopLogger(), c)

	resp := recorder.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("BearerToken test fails unexpectedly. Got: %s", body)
	}
}

func TestHTTPHeaders(t *testing.T) {
	headers := map[string]string{
		"X-Dummy":         "test",
		"User-Agent":      "unsuspicious user",
		"Accept-Language": "en-US",
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, value := range headers {
			if got := r.Header.Get(key); got != value {
				t.Errorf("Unexpected value of header %q: expected %q, got %q", key, value, got)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	req := httptest.NewRequest("GET", "http://example.com/foo"+"?target="+target.URL, nil)
	recorder := httptest.NewRecorder()
	c := config.Config{}
	c.Headers = headers

	probeHandler(recorder, req, log.NewNopLogger(), c)

	resp := recorder.Result()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Setting custom headers failed unexpectedly. Got: %s", body)
	}
}

// Test is the body template is correctly rendered
func TestBodyPostTemplate(t *testing.T) {
	bodyTests := []struct {
		Body          config.ConfigBody
		ShouldSucceed bool
		Result        string
	}{
		{
			Body:          config.ConfigBody{Content: "something static like pi, 3.14"},
			ShouldSucceed: true,
		},
		{
			Body:          config.ConfigBody{Content: "arbitrary dynamic value pass: {{ randInt 12 30 }}", Templatize: false},
			ShouldSucceed: true,
		},
		{
			Body:          config.ConfigBody{Content: "arbitrary dynamic value fail: {{ randInt 12 30 }}", Templatize: true},
			ShouldSucceed: false,
		},
		{
			Body:          config.ConfigBody{Content: "templatized mutated value: {{ upper `hello` }} is now all caps", Templatize: true},
			Result:        "templatized mutated value: HELLO is now all caps",
			ShouldSucceed: true,
		},
		{
			Body:          config.ConfigBody{Content: "value should be {{ lower `All Small` | trunc 3 }}", Templatize: true},
			Result:        "value should be all",
			ShouldSucceed: true,
		},
	}

	for _, test := range bodyTests {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			expected := test.Body.Content
			if test.Result != "" {
				expected = test.Result
			}
			if got, _ := io.ReadAll(r.Body); string(got) != expected && test.ShouldSucceed {
				t.Errorf("POST request body content mismatch, got: %s, expected: %s", got, expected)
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("POST", "http://example.com/foo"+"?target="+target.URL, strings.NewReader(test.Body.Content))
		recorder := httptest.NewRecorder()
		c := config.Config{Body: test.Body}

		probeHandler(recorder, req, log.NewNopLogger(), c)

		resp := recorder.Result()
		respBody, _ := ioutil.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST body content failed. Got: %s", respBody)
		}
		target.Close()
	}
}

// Test is the query parameters are correctly replaced in the provided body template
func TestBodyPostQuery(t *testing.T) {
	bodyTests := []struct {
		Body          config.ConfigBody
		ShouldSucceed bool
		Result        string
		QueryParams   map[string]string
	}{
		{
			Body:          config.ConfigBody{Content: "pi has {{ .piValue | first }} value", Templatize: true},
			ShouldSucceed: true,
			Result:        "pi has 3.14 value",
			QueryParams:   map[string]string{"piValue": "3.14"},
		},
		{
			Body:          config.ConfigBody{Content: `{ "pi": "{{ .piValue | first }}" }`, Templatize: true},
			ShouldSucceed: true,
			Result:        `{ "pi": "3.14" }`,
			QueryParams:   map[string]string{"piValue": "3.14"},
		},
		{
			Body:          config.ConfigBody{Content: "pi has {{ .anotherQuery | first }} value", Templatize: true},
			ShouldSucceed: true,
			Result:        "pi has very high value",
			QueryParams:   map[string]string{"piValue": "3.14", "anotherQuery": "very high"},
		},
		{
			Body:          config.ConfigBody{Content: "pi has {{ .piValue }} value", Templatize: true},
			ShouldSucceed: false,
			QueryParams:   map[string]string{"piValue": "3.14", "anotherQuery": "dummy value"},
		},
		{
			Body:          config.ConfigBody{Content: "pi has {{ .piValue }} value", Templatize: true},
			ShouldSucceed: true,
			Result:        "pi has [3.14] value",
			QueryParams:   map[string]string{"piValue": "3.14", "anotherQuery": "dummy value"},
		},
		{
			Body:          config.ConfigBody{Content: "value of {{ upper `pi` | repeat 3 }} is {{ .anotherQuery | first }}", Templatize: true},
			ShouldSucceed: true,
			Result:        "value of PIPIPI is dummy value",
			QueryParams:   map[string]string{"piValue": "3.14", "anotherQuery": "dummy value"},
		},
		{
			Body:          config.ConfigBody{Content: "pi has {{ .piValue }} value", Templatize: true},
			ShouldSucceed: true,
			Result:        "pi has [] value",
		},
		{
			Body:          config.ConfigBody{Content: "pi has {{ .piValue | first }} value", Templatize: true},
			ShouldSucceed: true,
			Result:        "pi has <no value> value",
		},
		{
			Body:          config.ConfigBody{Content: "value of pi is 3.14", Templatize: true},
			ShouldSucceed: true,
		},
	}

	for _, test := range bodyTests {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			expected := test.Body.Content
			if test.Result != "" {
				expected = test.Result
			}
			if got, _ := io.ReadAll(r.Body); string(got) != expected && test.ShouldSucceed {
				t.Errorf("POST request body content mismatch (with query params), got: %s, expected: %s", got, expected)
			}
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("POST", "http://example.com/foo"+"?target="+target.URL, strings.NewReader(test.Body.Content))
		q := req.URL.Query()
		for k, v := range test.QueryParams {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()

		recorder := httptest.NewRecorder()
		c := config.Config{Body: test.Body}

		probeHandler(recorder, req, log.NewNopLogger(), c)

		resp := recorder.Result()
		respBody, _ := ioutil.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST body content failed. Got: %s", respBody)
		}
		target.Close()
	}
}
