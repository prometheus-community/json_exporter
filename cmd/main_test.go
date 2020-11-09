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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
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

func TestTargetBaseConfig(t *testing.T) {
	test := struct {
		ConfigFile    string
		ServeFile     string
		ResponseFile  string
		ShouldSucceed bool
	}{"../test/config/good_tgt.yml", "/serve/good.json", "../test/response/good.txt", true}

	target := httptest.NewServer(http.FileServer(http.Dir("../test")))
	defer target.Close()

	c, err := config.LoadConfig(test.ConfigFile)
	if err != nil {
		t.Fatalf("Failed to load config file %s", test.ConfigFile)
	}
	if !strings.Contains(c.Target_base, "http://") {
		t.Fatalf("Target base config is not valid (test file wrong?)\nTarget_base: %s", c.Target_base)
	}

	for i := 0; i < 1; i++ {
		// Since valid, overwrite with the correct target
		var param_arg string
		if i == 0 {
			c.Target_base = target.URL + test.ServeFile
			param_arg = ""
		} else {
			c.Target_base = target.URL
			param_arg = "?params=" + test.ServeFile
		}
		req := httptest.NewRequest("GET", "http://example.com/foo"+param_arg, nil)
		recorder := httptest.NewRecorder()
		probeHandler(recorder, req, log.NewNopLogger(), c)

		resp := recorder.Result()
		body, _ := ioutil.ReadAll(resp.Body)

		expected, _ := ioutil.ReadFile(test.ResponseFile)

		if test.ShouldSucceed && string(body) != string(expected) {
			t.Fatalf("Correct response validation (target base config) test %d fails unexpectedly.\nGOT:\n%s\nEXPECTED:\n%s", i, body, expected)
		}
	}
}
