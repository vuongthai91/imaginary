package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const fixtureImage = "testdata/large.jpg"
const fixture1024Bytes = "testdata/1024bytes"

func TestHttpImageSource(t *testing.T) {
	var body []byte
	var err error

	buf, _ := ioutil.ReadFile(fixtureImage)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf)
	}))
	defer ts.Close()

	source := NewHTTPImageSource(&SourceConfig{})
	fakeHandler := func(w http.ResponseWriter, r *http.Request) {
		if !source.Matches(r) {
			t.Fatal("Cannot match the request")
		}

		body, err = source.GetImage(r)
		if err != nil {
			t.Fatalf("Error while reading the body: %s", err)
		}
		w.Write(body)
	}

	r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url="+ts.URL, nil)
	w := httptest.NewRecorder()
	fakeHandler(w, r)

	if len(body) != len(buf) {
		t.Error("Invalid response body")
	}
}

func TestHttpImageSourceAllowedOrigin(t *testing.T) {
	buf, _ := ioutil.ReadFile(fixtureImage)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf)
	}))
	defer ts.Close()

	origin, _ := url.Parse(ts.URL)
	origins := []*url.URL{origin}
	source := NewHTTPImageSource(&SourceConfig{AllowedOrigins: origins})

	fakeHandler := func(w http.ResponseWriter, r *http.Request) {
		if !source.Matches(r) {
			t.Fatal("Cannot match the request")
		}

		body, err := source.GetImage(r)
		if err != nil {
			t.Fatalf("Error while reading the body: %s", err)
		}
		w.Write(body)

		if len(body) != len(buf) {
			t.Error("Invalid response body length")
		}
	}

	r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url="+ts.URL, nil)
	w := httptest.NewRecorder()
	fakeHandler(w, r)
}

func TestHttpImageSourceNotAllowedOrigin(t *testing.T) {
	origin, _ := url.Parse("http://foo")
	origins := []*url.URL{origin}
	source := NewHTTPImageSource(&SourceConfig{AllowedOrigins: origins})

	fakeHandler := func(w http.ResponseWriter, r *http.Request) {
		if !source.Matches(r) {
			t.Fatal("Cannot match the request")
		}

		_, err := source.GetImage(r)
		if err == nil {
			t.Fatal("Error cannot be empty")
		}

		if err.Error() != "not allowed remote URL origin: bar.com" {
			t.Fatalf("Invalid error message: %s", err)
		}
	}

	r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url=http://bar.com", nil)
	w := httptest.NewRecorder()
	fakeHandler(w, r)
}

func TestHttpImageSourceForwardAuthHeader(t *testing.T) {
	cases := []string{
		"X-Forward-Authorization",
		"Authorization",
	}

	for _, header := range cases {
		r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url=http://bar.com", nil)
		r.Header.Set(header, "foobar")

		source := &HTTPImageSource{&SourceConfig{AuthForwarding: true}}
		if !source.Matches(r) {
			t.Fatal("Cannot match the request")
		}

		oreq := &http.Request{Header: make(http.Header)}
		source.setAuthorizationHeader(oreq, r)

		if oreq.Header.Get("Authorization") != "foobar" {
			t.Fatal("Missmatch Authorization header")
		}
	}
}

func TestHttpImageSourceForwardHeaders(t *testing.T) {
	cases := []string{
		"X-Custom",
		"X-Token",
	}

	for _, header := range cases {
		r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url=http://bar.com", nil)
		r.Header.Set(header, "foobar")

		source := &HTTPImageSource{&SourceConfig{ForwardHeaders: cases}}
		if !source.Matches(r) {
			t.Fatal("Cannot match the request")
		}

		oreq := &http.Request{Header: make(http.Header)}
		source.setForwardHeaders(oreq, r)

		if oreq.Header.Get(header) != "foobar" {
			t.Fatal("Missmatch custom header")
		}
	}
}

func TestHttpImageSourceNotForwardHeaders(t *testing.T) {
	cases := []string{
		"X-Custom",
		"X-Token",
	}

	url := createURL("http://bar.com", t)

	r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url="+url.String(), nil)
	r.Header.Set("Not-Forward", "foobar")

	source := &HTTPImageSource{&SourceConfig{ForwardHeaders: cases}}
	if !source.Matches(r) {
		t.Fatal("Cannot match the request")
	}

	oreq := newHTTPRequest(source, r, http.MethodGet, url)

	if oreq.Header.Get("Not-Forward") != "" {
		t.Fatal("Forwarded unspecified header")
	}
}

func TestHttpImageSourceForwardedHeadersNotOverride(t *testing.T) {
	cases := []string{
		"Authorization",
		"X-Custom",
	}

	url := createURL("http://bar.com", t)

	r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url="+url.String() , nil)
	r.Header.Set("Authorization", "foobar")
	
	source := &HTTPImageSource{&SourceConfig{Authorization: "ValidAPIKey", ForwardHeaders: cases}}
	if !source.Matches(r) {
		t.Fatal("Cannot match the request")
	}

	oreq := newHTTPRequest(source, r, http.MethodGet, url)

	if oreq.Header.Get("Authorization") != "ValidAPIKey" {
		t.Fatal("Authorization header override")
	}
}

func TestHttpImageSourceCaseSensitivityInForwardedHeaders(t *testing.T) {
	cases := []string{
		"X-Custom",
		"X-Token",
	}

	url := createURL("http://bar.com", t)

	r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url="+url.String(), nil)
	r.Header.Set("x-custom", "foobar")

	source := &HTTPImageSource{&SourceConfig{ForwardHeaders: cases}}
	if !source.Matches(r) {
		t.Fatal("Cannot match the request")
	}

	oreq := newHTTPRequest(source, r, http.MethodGet, url)

	if oreq.Header.Get("X-Custom") == "" {
		t.Fatal("Case sensitive not working on forwarded headers")
	}
}

func TestHttpImageSourceEmptyForwardedHeaders(t *testing.T) {
	cases := []string{}

	url := createURL("http://bar.com", t)

	r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url="+url.String(), nil)

	source := &HTTPImageSource{&SourceConfig{ForwardHeaders: cases}}
	if !source.Matches(r) {
		t.Fatal("Cannot match the request")
	}

	if len(source.Config.ForwardHeaders) != 0 {
		t.Log(source.Config.ForwardHeaders)
		t.Fatal("Setted empty custom header")
	}

	oreq := newHTTPRequest(source, r, http.MethodGet, url)

	if oreq == nil {
		t.Fatal("Error creating request using empty custom headers")
	}
}

func TestHttpImageSourceError(t *testing.T) {
	var err error

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("Not found"))
	}))
	defer ts.Close()

	source := NewHTTPImageSource(&SourceConfig{})
	fakeHandler := func(w http.ResponseWriter, r *http.Request) {
		if !source.Matches(r) {
			t.Fatal("Cannot match the request")
		}

		_, err = source.GetImage(r)
		if err == nil {
			t.Fatalf("Server response should not be valid: %s", err)
		}
	}

	r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url="+ts.URL, nil)
	w := httptest.NewRecorder()
	fakeHandler(w, r)
}

func TestHttpImageSourceExceedsMaximumAllowedLength(t *testing.T) {
	var body []byte
	var err error

	buf, _ := ioutil.ReadFile(fixture1024Bytes)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf)
	}))
	defer ts.Close()

	source := NewHTTPImageSource(&SourceConfig{
		MaxAllowedSize: 1023,
	})
	fakeHandler := func(w http.ResponseWriter, r *http.Request) {
		if !source.Matches(r) {
			t.Fatal("Cannot match the request")
		}

		body, err = source.GetImage(r)
		if err == nil {
			t.Fatalf("It should not allow a request to image exceeding maximum allowed size: %s", err)
		}
		w.Write(body)
	}

	r, _ := http.NewRequest(http.MethodGet, "http://foo/bar?url="+ts.URL, nil)
	w := httptest.NewRecorder()
	fakeHandler(w, r)
}

func TestShouldRestrictOrigin(t *testing.T) {
	plainOrigins := []*url.URL{
		createURL("https://example.org", t),
	}

	wildCardOrigins := []*url.URL{
		createURL("https://localhost", t),
		createURL("https://*.example.org", t),
		createURL("https://some.s3.bucket.on.aws.org", t),
		createURL("https://*.s3.bucket.on.aws.org", t),
	}

	t.Run("Plain origin", func(t *testing.T) {
		testURL := createURL("https://example.org/logo.jpg", t)

		if shouldRestrictOrigin(testURL, plainOrigins) {
			t.Errorf("Expected '%s' to be allowed with origins: %+v", testURL, plainOrigins)
		}
	})

	t.Run("Wildcard origin, plain URL", func(t *testing.T) {
		testURL := createURL("https://example.org/logo.jpg", t)

		if shouldRestrictOrigin(testURL, wildCardOrigins) {
			t.Errorf("Expected '%s' to be allowed with origins: %+v", testURL, wildCardOrigins)
		}
	})

	t.Run("Wildcard origin, sub domain URL", func(t *testing.T) {
		testURL := createURL("https://node-42.example.org/logo.jpg", t)

		if shouldRestrictOrigin(testURL, wildCardOrigins) {
			t.Errorf("Expected '%s' to be allowed with origins: %+v", testURL, wildCardOrigins)
		}
	})

	t.Run("Wildcard origin, sub-sub domain URL", func(t *testing.T) {
		testURL := createURL("https://n.s3.bucket.on.aws.org/logo.jpg", t)

		if shouldRestrictOrigin(testURL, wildCardOrigins) {
			t.Errorf("Expected '%s' to be allowed with origins: %+v", testURL, wildCardOrigins)
		}
	})

	t.Run("Wildcard origin, incorrect domain URL", func(t *testing.T) {
		testURL := createURL("https://myexample.org/logo.jpg", t)

		if !shouldRestrictOrigin(testURL, plainOrigins) {
			t.Errorf("Expected '%s' to not be allowed with plain origins: %+v", testURL, plainOrigins)
		}

		if !shouldRestrictOrigin(testURL, wildCardOrigins) {
			t.Errorf("Expected '%s' to not be allowed with wildcard origins: %+v", testURL, wildCardOrigins)
		}
	})
}

func createURL(urlStr string, t *testing.T) *url.URL {
	t.Helper()

	result, err := url.Parse(urlStr)

	if err != nil {
		t.Error("Test setup failed, unable to parse test URL")
	}

	return result
}
