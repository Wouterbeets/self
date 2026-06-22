package seed

import (
	"encoding/json"
	"testing"
)

func TestCommandCanonicalSchema(t *testing.T) {
	raw := `{"name":"note","description":"capture","params":{"title":"string"},"event":{"name":"note.captured","fields":{"title":"string"}}}`
	var cmd Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "note" {
		t.Errorf("name = %q", cmd.Name)
	}
	if cmd.Event.Name != "note.captured" {
		t.Errorf("event name = %q", cmd.Event.Name)
	}
}

func TestCommandLLMSchemaParamsArray(t *testing.T) {
	raw := `{"command":"todo","description":"add todo","params":[{"name":"title","type":"string","required":true}],"event":{"name":"task.created","fields":{"title":"string","done":"boolean"}}}`
	var cmd Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "todo" {
		t.Errorf("name = %q, want todo", cmd.Name)
	}
	if len(cmd.Params) != 1 || cmd.Params["title"] != "string" {
		t.Errorf("params = %v, want {title: string}", cmd.Params)
	}
}

func TestCommandLLMSchemaEventAsString(t *testing.T) {
	raw := `{"name":"todo","description":"add todo","params":{},"event":"task.created","fields":{"title":{"type":"string"},"done":{"type":"boolean","default":false}}}`
	var cmd Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Event.Name != "task.created" {
		t.Errorf("event name = %q, want task.created", cmd.Event.Name)
	}
	if cmd.Event.Fields["title"] != "string" {
		t.Errorf("field title = %q, want string", cmd.Event.Fields["title"])
	}
	if cmd.Event.Fields["done"] != "boolean" {
		t.Errorf("field done = %q, want boolean", cmd.Event.Fields["done"])
	}
}

func TestProjectorDeclCanonicalSchema(t *testing.T) {
	raw := `{"name":"notes","description":"render notes","consumes":["note.captured"]}`
	var proj ProjectorDecl
	if err := json.Unmarshal([]byte(raw), &proj); err != nil {
		t.Fatal(err)
	}
	if proj.Name != "notes" {
		t.Errorf("name = %q", proj.Name)
	}
	if len(proj.Consumes) != 1 || proj.Consumes[0] != "note.captured" {
		t.Errorf("consumes = %v", proj.Consumes)
	}
}

func TestProjectorDeclLLMSchemaConsumesObjects(t *testing.T) {
	raw := `{"name":"todos","description":"render todos","consumes":[{"event":"task.created"},{"name":"task.completed"}]}`
	var proj ProjectorDecl
	if err := json.Unmarshal([]byte(raw), &proj); err != nil {
		t.Fatal(err)
	}
	if proj.Name != "todos" {
		t.Errorf("name = %q", proj.Name)
	}
	if len(proj.Consumes) != 2 {
		t.Fatalf("consumes len = %d, want 2", len(proj.Consumes))
	}
	if proj.Consumes[0] != "task.created" || proj.Consumes[1] != "task.completed" {
		t.Errorf("consumes = %v", proj.Consumes)
	}
}

func TestCommandLLMSchemaFromRealOutput(t *testing.T) {
	raw := `{
		"command": "todo",
		"description": "Add a todo task. Usage: todo <title...>",
		"params": [{"name": "title", "type": "text", "required": true, "join": "argv"}],
		"event": "task.created",
		"fields": {"title": {"type": "string", "source": "argv"}, "done": {"type": "boolean", "default": false}}
	}`
	var cmd Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatalf("should parse real LLM output: %v", err)
	}
	if cmd.Name != "todo" {
		t.Errorf("name = %q, want todo", cmd.Name)
	}
	if cmd.Params["title"] != "text" {
		t.Errorf("params[title] = %q, want text", cmd.Params["title"])
	}
	if cmd.Event.Name != "task.created" {
		t.Errorf("event name = %q, want task.created", cmd.Event.Name)
	}
	if cmd.Event.Fields["done"] != "boolean" {
		t.Errorf("field done = %q, want boolean", cmd.Event.Fields["done"])
	}
}
