package seed

import (
	"encoding/json"
	"fmt"
)

// UnmarshalJSON for Command accepts both the canonical schema:
//
//	{"name": "...", "params": {"key": "type"}, "event": {"name": "...", "fields": {"k": "v"}}}
//
// and LLM-generated variants:
//
//	{"command": "...", "params": [{"name": "k", "type": "v"}], "event": "name", "fields": {"k": {"type": "v"}}}
func (c *Command) UnmarshalJSON(data []byte) error {
	type canonical Command
	var can canonical
	if err := json.Unmarshal(data, &can); err == nil && can.Name != "" {
		*c = Command(can)
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if v, ok := rawFieldString(raw, "name"); ok {
		c.Name = v
	} else if v, ok := rawFieldString(raw, "command"); ok {
		c.Name = v
	}

	if v, ok := rawFieldString(raw, "description"); ok {
		c.Description = v
	}
	if v, ok := rawFieldString(raw, "implementation"); ok {
		c.Implementation = v
	}

	c.Params = parseParamsLenient(raw["params"])
	c.Event = parseEventLenient(raw["event"], raw["fields"])

	return nil
}

// UnmarshalJSON for EventDecl accepts fields as map[string]string or
// map[string]{"type": "string", ...} (LLM-generated nested objects).
func (e *EventDecl) UnmarshalJSON(data []byte) error {
	type canonical EventDecl
	var can canonical
	if err := json.Unmarshal(data, &can); err == nil && can.Name != "" {
		*e = EventDecl(can)
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if v, ok := rawFieldString(raw, "name"); ok {
		e.Name = v
	}
	if v, ok := rawFieldString(raw, "event"); ok {
		e.Name = v
	}

	e.Fields = parseFieldsLenient(raw["fields"])

	return nil
}

// UnmarshalJSON for ProjectorDecl accepts consumes as []string or
// [{"event": "name"}, {"name": "name"}] (LLM-generated array of objects).
func (p *ProjectorDecl) UnmarshalJSON(data []byte) error {
	type canonical ProjectorDecl
	var can canonical
	if err := json.Unmarshal(data, &can); err == nil && can.Name != "" {
		*p = ProjectorDecl(can)
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if v, ok := rawFieldString(raw, "name"); ok {
		p.Name = v
	} else if v, ok := rawFieldString(raw, "projector"); ok {
		p.Name = v
	}

	if v, ok := rawFieldString(raw, "description"); ok {
		p.Description = v
	}
	if v, ok := rawFieldString(raw, "implementation"); ok {
		p.Implementation = v
	}

	p.Consumes = parseConsumesLenient(raw["consumes"], raw["events"])

	return nil
}

func rawFieldString(raw map[string]json.RawMessage, key string) (string, bool) {
	v, ok := raw[key]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return "", false
	}
	return s, true
}

// parseParamsLenient accepts:
//
//	{"key": "type string"}                    → map
//	[{"name": "key", "type": "type"}]         → array of objects
//	[{"name": "key", "type": "type", ...}]    → array with extra fields
func parseParamsLenient(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}

	var m map[string]string
	if err := json.Unmarshal(raw, &m); err == nil {
		return m
	}

	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil {
		params := map[string]string{}
		for _, item := range arr {
			name, _ := item["name"].(string)
			if name == "" {
				continue
			}
			typeStr, _ := item["type"].(string)
			if typeStr == "" {
				typeStr = "string"
			}
			params[name] = typeStr
		}
		if len(params) > 0 {
			return params
		}
	}

	return nil
}

// parseFieldsLenient accepts:
//
//	{"key": "type string"}                    → map
//	{"key": {"type": "string", ...}}          → nested objects
func parseFieldsLenient(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}

	var m map[string]string
	if err := json.Unmarshal(raw, &m); err == nil {
		return m
	}

	var nested map[string]map[string]any
	if err := json.Unmarshal(raw, &nested); err == nil {
		fields := map[string]string{}
		for k, v := range nested {
			if typeStr, ok := v["type"].(string); ok {
				fields[k] = typeStr
			} else {
				fields[k] = "string"
			}
		}
		if len(fields) > 0 {
			return fields
		}
	}

	return nil
}

// parseEventLenient accepts:
//
//	{"name": "...", "fields": {...}}  → object
//	"event.name"                      → string (fields from separate key)
func parseEventLenient(eventRaw, fieldsRaw json.RawMessage) EventDecl {
	var ed EventDecl
	if len(eventRaw) > 0 {
		if err := json.Unmarshal(eventRaw, &ed); err == nil && ed.Name != "" {
			if len(fieldsRaw) > 0 && len(ed.Fields) == 0 {
				ed.Fields = parseFieldsLenient(fieldsRaw)
			}
			return ed
		}

		var s string
		if err := json.Unmarshal(eventRaw, &s); err == nil {
			ed.Name = s
		}
	}

	if len(fieldsRaw) > 0 {
		ed.Fields = parseFieldsLenient(fieldsRaw)
	}

	return ed
}

// parseConsumesLenient accepts:
//
//	["event.a", "event.b"]                    → string array
//	[{"event": "name"}, {"name": "name"}]     → array of objects
func parseConsumesLenient(consumesRaw, eventsRaw json.RawMessage) []string {
	raw := consumesRaw
	if len(raw) == 0 {
		raw = eventsRaw
	}
	if len(raw) == 0 {
		return nil
	}

	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}

	var objArr []map[string]any
	if err := json.Unmarshal(raw, &objArr); err == nil {
		var consumes []string
		for _, item := range objArr {
			if name, ok := item["event"].(string); ok {
				consumes = append(consumes, name)
			} else if name, ok := item["name"].(string); ok {
				consumes = append(consumes, name)
			}
		}
		if len(consumes) > 0 {
			return consumes
		}
	}

	return nil
}

var _ = fmt.Sprintf
