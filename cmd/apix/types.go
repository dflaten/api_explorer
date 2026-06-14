package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

type ValueEntry struct {
	Key   string
	Value any
}

type OrderedValues struct {
	Entries []ValueEntry
}

func (values *OrderedValues) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("must be an object")
	}
	for index := 0; index < len(node.Content); index += 2 {
		var value any
		if err := node.Content[index+1].Decode(&value); err != nil {
			return err
		}
		values.Set(node.Content[index].Value, value)
	}
	return nil
}

func (values *OrderedValues) Set(key string, value any) {
	for index := range values.Entries {
		if values.Entries[index].Key == key {
			values.Entries[index].Value = value
			return
		}
	}
	values.Entries = append(values.Entries, ValueEntry{Key: key, Value: value})
}

func (values *OrderedValues) Delete(key string) (any, bool) {
	for index, entry := range values.Entries {
		if entry.Key == key {
			values.Entries = append(values.Entries[:index], values.Entries[index+1:]...)
			return entry.Value, true
		}
	}
	return nil, false
}

func (values OrderedValues) Clone() OrderedValues {
	return OrderedValues{Entries: append([]ValueEntry(nil), values.Entries...)}
}

func (values OrderedValues) Len() int {
	return len(values.Entries)
}

func (values OrderedValues) Map() map[string]any {
	result := make(map[string]any, len(values.Entries))
	for _, entry := range values.Entries {
		result[entry.Key] = entry.Value
	}
	return result
}

func parseOrderedJSON(value string) (OrderedValues, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	token, err := decoder.Token()
	if err != nil {
		return OrderedValues{}, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '{' {
		return OrderedValues{}, fmt.Errorf("must be a JSON object")
	}
	result := OrderedValues{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return OrderedValues{}, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return OrderedValues{}, fmt.Errorf("object key must be a string")
		}
		var item any
		if err := decoder.Decode(&item); err != nil {
			return OrderedValues{}, err
		}
		result.Set(key, item)
	}
	if _, err := decoder.Token(); err != nil {
		return OrderedValues{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return OrderedValues{}, fmt.Errorf("unexpected trailing data")
	} else if err != io.EOF {
		return OrderedValues{}, err
	}
	return result, nil
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case nil:
		return "None"
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

type Config struct {
	BaseURL        string               `yaml:"base_url"`
	Timeout        float64              `yaml:"timeout"`
	DefaultHeaders map[string]string    `yaml:"default_headers"`
	Auth           *Auth                `yaml:"auth"`
	Endpoints      map[string]*Endpoint `yaml:"endpoints"`
	EndpointOrder  []string             `yaml:"-"`
}

type Auth struct {
	Type     string `yaml:"type"`
	Token    string `yaml:"token"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Endpoint struct {
	Method      string            `yaml:"method"`
	Path        *string           `yaml:"path"`
	URL         *string           `yaml:"url"`
	BaseURL     string            `yaml:"base_url"`
	Description string            `yaml:"description"`
	Headers     map[string]string `yaml:"headers"`
	Params      OrderedValues     `yaml:"params"`
	Body        any               `yaml:"body"`
	BodyType    string            `yaml:"body_type"`
}

type Collection struct {
	Requests []CollectionRequest `yaml:"requests"`
}

type CollectionRequest struct {
	Endpoint string            `yaml:"endpoint"`
	BodyFile string            `yaml:"body_file"`
	Params   OrderedValues     `yaml:"params"`
	Headers  map[string]string `yaml:"headers"`
}

type RequestDefinition struct {
	Endpoint         string
	Definition       *Endpoint
	Method           string
	FullURL          string
	Params           OrderedValues
	EffectiveHeaders map[string]string
	Timeout          float64
	Body             any
	BodyType         string
}
