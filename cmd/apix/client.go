package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var pathParameterPattern = regexp.MustCompile(`\{([^{}]+)\}`)

// APIClient builds and executes requests from a loaded API configuration.
type APIClient struct {
	ConfigPath string
	Config     *Config
	HTTP       *http.Client
}

func newAPIClient(configPath string) (*APIClient, error) {
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}
	return &APIClient{
		ConfigPath: configPath,
		Config:     config,
		HTTP:       &http.Client{Timeout: time.Duration(config.Timeout * float64(time.Second))},
	}, nil
}

func (client *APIClient) endpoint(name string) (*Endpoint, error) {
	endpoint, ok := client.Config.Endpoints[name]
	if !ok {
		return nil, fmt.Errorf("Endpoint '%s' not found in config", name)
	}
	return endpoint, nil
}

func (client *APIClient) buildRequest(name, bodyPath string, parameterOverrides OrderedValues, headerOverrides map[string]string) (*RequestDefinition, error) {
	endpoint, err := client.endpoint(name)
	if err != nil {
		return nil, err
	}
	parameters := endpoint.Params.Clone()
	for _, entry := range parameterOverrides.Entries {
		parameters.Set(entry.Key, entry.Value)
	}

	requestURL := ""
	pathParameters := OrderedValues{}
	if endpoint.URL != nil {
		requestURL = *endpoint.URL
	} else {
		path := *endpoint.Path
		for _, match := range pathParameterPattern.FindAllStringSubmatch(path, -1) {
			if value, found := parameters.Delete(match[1]); found {
				pathParameters.Set(match[1], value)
				path = strings.ReplaceAll(path, match[0], url.PathEscape(scalarString(value)))
			}
		}
		unresolved := pathParameterPattern.FindAllStringSubmatch(path, -1)
		if len(unresolved) > 0 {
			names := make([]string, 0, len(unresolved))
			for _, match := range unresolved {
				names = append(names, match[1])
			}
			return nil, fmt.Errorf("missing path parameter values: %s", strings.Join(names, ", "))
		}
		baseURL := endpoint.BaseURL
		if baseURL == "" {
			baseURL = client.Config.BaseURL
		}
		requestURL = baseURL + path
	}

	fullURL := requestURL
	if parameters.Len() > 0 {
		parsedURL, err := url.Parse(requestURL)
		if err != nil {
			return nil, err
		}
		encodedParameters := encodeValues(parameters)
		if parsedURL.RawQuery == "" {
			parsedURL.RawQuery = encodedParameters
		} else {
			parsedURL.RawQuery += "&" + encodedParameters
		}
		fullURL = parsedURL.String()
	}

	requestHeaders := cloneHeaders(endpoint.Headers)
	for key, value := range headerOverrides {
		requestHeaders[key] = value
	}
	effectiveHeaders := cloneHeaders(client.Config.DefaultHeaders)
	auth := client.Config.Auth
	if endpoint.Auth != nil {
		auth = endpoint.Auth
	}
	applyAuth(effectiveHeaders, auth)
	for key, value := range requestHeaders {
		effectiveHeaders[key] = value
	}

	body := endpoint.Body
	if bodyPath != "" {
		data, err := os.ReadFile(bodyPath)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return nil, err
		}
	}
	bodyType := endpoint.BodyType
	if bodyType == "" {
		bodyType = "json"
	}
	return &RequestDefinition{
		Endpoint:         name,
		Definition:       endpoint,
		Method:           strings.ToUpper(endpoint.Method),
		FullURL:          fullURL,
		Path:             requestLogPath(redactURL(fullURL)),
		PathParams:       pathParameters,
		QueryParams:      parameters,
		EffectiveHeaders: effectiveHeaders,
		Timeout:          client.Config.Timeout,
		Body:             body,
		BodyType:         bodyType,
	}, nil
}

func (client *APIClient) execute(definition *RequestDefinition) (*http.Response, error) {
	var body io.Reader
	if definition.Body != nil {
		if definition.BodyType == "form" {
			values := url.Values{}
			if itemMap, ok := definition.Body.(map[string]any); ok {
				for key, value := range itemMap {
					values.Set(key, scalarString(value))
				}
			}
			body = strings.NewReader(values.Encode())
		} else {
			encoded, err := json.Marshal(definition.Body)
			if err != nil {
				return nil, err
			}
			body = bytes.NewReader(encoded)
		}
	}
	request, err := http.NewRequest(definition.Method, definition.FullURL, body)
	if err != nil {
		return nil, err
	}
	for key, value := range definition.EffectiveHeaders {
		request.Header[key] = []string{value}
	}
	if definition.Body != nil && request.Header.Get("Content-Type") == "" {
		if definition.BodyType == "form" {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			request.Header.Set("Content-Type", "application/json")
		}
	}
	return client.HTTP.Do(request)
}

func parseResponse(response *http.Response) (any, []byte, error) {
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, err
	}
	if len(data) == 0 {
		return nil, data, nil
	}
	var value any
	if json.Unmarshal(data, &value) == nil {
		return value, data, nil
	}
	return string(data), data, nil
}

func encodeValues(values OrderedValues) string {
	parts := make([]string, 0, values.Len())
	for _, entry := range values.Entries {
		parts = append(parts, url.QueryEscape(entry.Key)+"="+url.QueryEscape(scalarString(entry.Value)))
	}
	return strings.Join(parts, "&")
}

func cloneHeaders(headers map[string]string) map[string]string {
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		result[key] = value
	}
	return result
}

func applyAuth(headers map[string]string, auth *Auth) {
	if auth == nil {
		return
	}
	if auth.Type == "bearer" {
		headers["Authorization"] = "Bearer " + auth.Token
	} else if auth.Type == "basic" {
		credentials := base64.StdEncoding.EncodeToString([]byte(auth.Username + ":" + auth.Password))
		headers["Authorization"] = "Basic " + credentials
	}
}

func responseHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		result[key] = strings.Join(values, ", ")
	}
	return result
}

func (client *APIClient) executeCollection(path string) ([]map[string]any, error) {
	collection, err := loadCollection(path)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, len(collection.Requests))
	for _, item := range collection.Requests {
		definition, buildErr := client.buildRequest(item.Endpoint, item.BodyFile, item.Params, item.Headers)
		if buildErr != nil {
			results = append(results, map[string]any{
				"endpoint": item.Endpoint,
				"success":  false,
				"error":    buildErr.Error(),
			})
			continue
		}
		response, requestErr := executeWithLog(client, definition)
		if requestErr != nil {
			results = append(results, map[string]any{
				"endpoint": item.Endpoint,
				"success":  false,
				"error":    requestErr.Error(),
			})
			continue
		}
		parsed, _, parseErr := parseResponse(response)
		response.Body.Close()
		if parseErr != nil {
			results = append(results, map[string]any{
				"endpoint": item.Endpoint,
				"success":  false,
				"error":    parseErr.Error(),
			})
			continue
		}
		results = append(results, map[string]any{
			"endpoint":    item.Endpoint,
			"status_code": response.StatusCode,
			"success":     response.StatusCode >= 200 && response.StatusCode < 400,
			"response":    parsed,
			"headers":     responseHeaders(response.Header),
		})
	}
	return results, nil
}
