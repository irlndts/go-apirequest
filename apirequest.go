package apirequest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/go-querystring/query"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

const (
	contentType     = "Content-Type"
	jsonContentType = "application/json"
	formContentType = "application/x-www-form-urlencoded"
)

type API struct {
	httpClient   *http.Client
	method       string
	rawURL       string
	header       http.Header
	queryStructs []interface{}
	bodyJSON     interface{}
	bodyForm     interface{}
	body         io.ReadCloser
}

func New() *API {
	return &API{
		httpClient:   http.DefaultClient,
		method:       "GET",
		header:       make(http.Header),
		queryStructs: make([]interface{}, 0),
	}
}

func (s *API) New() *API {
	// copy Headers pairs into new Header map
	headerCopy := make(http.Header)
	for k, v := range s.header {
		headerCopy[k] = v
	}
	return &API{
		httpClient:   s.httpClient,
		method:       s.method,
		rawURL:       s.rawURL,
		header:       headerCopy,
		queryStructs: append([]interface{}{}, s.queryStructs...),
		bodyJSON:     s.bodyJSON,
		bodyForm:     s.bodyForm,
		body:         s.body,
	}
}

func (s *API) Client(httpClient *http.Client) *API {
	if httpClient == nil {
		s.httpClient = http.DefaultClient
	} else {
		s.httpClient = httpClient
	}
	return s
}

func (s *API) Get(pathURL string) *API {
	s.method = "GET"
	return s.Path(pathURL)
}

func (s *API) Post(pathURL string) *API {
	s.method = "POST"
	return s.Path(pathURL)
}

// Header

func (s *API) Add(key, value string) *API {
	s.header.Add(key, value)
	return s
}

// Replace key->value
func (s *API) Set(key, value string) *API {
	s.header.Set(key, value)
	return s
}

// Base sets the base URL. If you intend to extend the url with Path,
func (s *API) Base(rawURL string) *API {
	s.rawURL = rawURL
	return s
}

// Path extends the rawURL with the given path by resolving the reference to
func (s *API) Path(path string) *API {
	baseURL, baseErr := url.Parse(s.rawURL)
	pathURL, pathErr := url.Parse(path)
	if baseErr == nil && pathErr == nil {
		s.rawURL = baseURL.ResolveReference(pathURL).String()
		return s
	}
	return s
}

func (s *API) QueryStruct(queryStruct interface{}) *API {
	if queryStruct != nil {
		s.queryStructs = append(s.queryStructs, queryStruct)
	}
	return s
}

func (s *API) BodyJSON(bodyJSON interface{}) *API {
	if bodyJSON != nil {
		s.bodyJSON = bodyJSON
		s.Set(contentType, jsonContentType)
	}
	return s
}

func (s *API) BodyForm(bodyForm interface{}) *API {
	if bodyForm != nil {
		s.bodyForm = bodyForm
		s.Set(contentType, formContentType)
	}
	return s
}

func (s *API) Body(body io.Reader) *API {
	rc, ok := body.(io.ReadCloser)
	if !ok && body != nil {
		rc = ioutil.NopCloser(body)
	}
	if rc != nil {
		s.body = rc
	}
	return s
}

////////////
//Requests//
////////////

func (s *API) Request() (*http.Request, error) {
	reqURL, err := url.Parse(s.rawURL)
	if err != nil {
		return nil, err
	}
	err = addQueryStructs(reqURL, s.queryStructs)
	fmt.Println(reqURL)
	if err != nil {
		return nil, err
	}
	body, err := s.getRequestBody()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(s.method, reqURL.String(), body)
	if err != nil {
		return nil, err
	}
	addHeaders(req, s.header)
	return req, err
}

func addQueryStructs(reqURL *url.URL, queryStructs []interface{}) error {
	urlValues, err := url.ParseQuery(reqURL.RawQuery)
	if err != nil {
		return err
	}
	// encodes query structs into a url.Values map and merges maps
	for _, queryStruct := range queryStructs {
		queryValues, err := query.Values(queryStruct)
		if err != nil {
			return err
		}
		for key, values := range queryValues {
			for _, value := range values {
				urlValues.Add(strings.ToLower(key), value)
			}
		}
	}
	// url.Values format to a sorted "url encoded" string, e.g. "key=val&foo=bar"
	reqURL.RawQuery = urlValues.Encode()
	return nil
}

func (s *API) getRequestBody() (body io.Reader, err error) {
	if s.bodyJSON != nil && s.header.Get(contentType) == jsonContentType {
		body, err = encodeBodyJSON(s.bodyJSON)
		if err != nil {
			return nil, err
		}
	} else if s.bodyForm != nil && s.header.Get(contentType) == formContentType {
		body, err = encodeBodyForm(s.bodyForm)
		if err != nil {
			return nil, err
		}
	} else if s.body != nil {
		body = s.body
	}
	return body, nil
}

func encodeBodyJSON(bodyJSON interface{}) (io.Reader, error) {
	var buf = new(bytes.Buffer)
	if bodyJSON != nil {
		buf = &bytes.Buffer{}
		err := json.NewEncoder(buf).Encode(bodyJSON)
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}

func encodeBodyForm(bodyForm interface{}) (io.Reader, error) {
	values, err := query.Values(bodyForm)
	if err != nil {
		return nil, err
	}
	return strings.NewReader(values.Encode()), nil
}

func addHeaders(req *http.Request, header http.Header) {
	for key, values := range header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
}

// Sending

func (s *API) ReceiveSuccess(successV interface{}) (*http.Response, error) {
	return s.Receive(successV, nil)
}

func (s *API) Receive(successV, failureV interface{}) (*http.Response, error) {
	req, err := s.Request()
	if err != nil {
		return nil, err
	}
	return s.Do(req, successV, failureV)
}

func (s *API) Do(req *http.Request, successV, failureV interface{}) (*http.Response, error) {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return resp, err
	}
	// when err is nil, resp contains a non-nil resp.Body which must be closed
	defer resp.Body.Close()
	if strings.Contains(resp.Header.Get(contentType), jsonContentType) {
		err = decodeResponseJSON(resp, successV, failureV)
	}
	return resp, err
}

func decodeResponseJSON(resp *http.Response, successV, failureV interface{}) error {
	if code := resp.StatusCode; 200 <= code && code <= 299 {
		if successV != nil {
			return decodeResponseBodyJSON(resp, successV)
		}
	} else {
		if failureV != nil {
			return decodeResponseBodyJSON(resp, failureV)
		}
	}
	return nil
}

func decodeResponseBodyJSON(resp *http.Response, v interface{}) error {
	return json.NewDecoder(resp.Body).Decode(v)
}
