package httpread

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/bpicode/fritzctl/logger"
	"github.com/pkg/errors"
)

var (
	httpStatusBuzzwords = map[string]int{"500 Internal Server Error": 500}
)

// HTTPStatusCodeError represents an 4xx client or a 5xx server error.
type HTTPStatusCodeError struct {
	error
}

func statusCodeError(code int, phrase string) *HTTPStatusCodeError {
	return &HTTPStatusCodeError{error: fmt.Errorf("HTTP status code error (%d): remote replied with '%s'", code, phrase)}
}

// ReadFullyString reads a http response into a string.
// The response is checked for its status code and the http.Response.Body is closed.
func ReadFullyString(f func() (*http.Response, error)) (string, error) {
	response, err := f()
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	bytesRead, err := ioutil.ReadAll(response.Body)
	body := string(bytesRead)
	logger.Debug("DATA:", body)
	statusCode, statusPhrase := guessStatusCode(response.StatusCode, response.Status, body)
	if statusCode >= 400 {
		return body, statusCodeError(statusCode, statusPhrase)
	}
	return body, err
}

func guessStatusCode(claimedCode int, claimedPhrase, body string) (int, string) {
	if claimedCode >= 400 {
		return claimedCode, claimedPhrase // This is already bad enough.
	}
	// There are web servers that send the wrong status code, but provide some hint in the text/html.
	for k, v := range httpStatusBuzzwords {
		if strings.Contains(strings.ToLower(body), strings.ToLower(k)) {
			return v, k
		}
	}
	return claimedCode, claimedPhrase
}

// DecodeError represents an error related to unmarshalling.
type DecodeError struct {
	error
}

func decodeError(err error) *DecodeError {
	return &DecodeError{error: errors.Wrap(err, "unable to parse remote response")}
}

type decoder interface {
	Decode(v interface{}) error
}

type decoderFactory func(io.Reader) decoder

// ReadFullyXML reads a http response into a data container using an XML decoder.
// The response is checked for its status code and the http.Response.Body is closed.
func ReadFullyXML(f func() (*http.Response, error), v interface{}) error {
	return readDecode(f, func(r io.Reader) decoder {
		return xml.NewDecoder(r)
	}, v)
}

// ReadFullyJSON reads a http response into a data container using a json decoder.
// The response is checked for its status code and the http.Response.Body is closed.
func ReadFullyJSON(f func() (*http.Response, error), v interface{}) error {
	return readDecode(f, func(r io.Reader) decoder {
		return json.NewDecoder(r)
	}, v)
}

func readDecode(f func() (*http.Response, error), df decoderFactory, v interface{}) error {
	response, err := f()
	if err != nil {
		return errors.Wrap(err, "error obtaining HTTP response from remote")
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		return statusCodeError(response.StatusCode, response.Status)
	}
	return decode(response.Body, df, v)
}

func decode(r io.Reader, df decoderFactory, v interface{}) error {
	buf := new(bytes.Buffer)
	tee := io.TeeReader(r, buf)
	defer func() { logger.Debug("DATA:", buf) }()
	err := df(tee).Decode(v)
	if err != nil {
		return decodeError(err)
	}
	return nil
}
