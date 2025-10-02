// Package agui was used by a legacy Agent-UI WebSocket and is now removed.
// This file remains temporarily to avoid breaking imports, but will be deleted.
package agui

// RunStartedEvent signals the start of a run.
type RunStartedEvent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TextMessageEvent for log/text output.
type TextMessageEvent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolCallEvent for tool invocations.
type ToolCallEvent struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}

// RunFinishedEvent signals completion.
type RunFinishedEvent struct {
	Output string `json:"output"`
}
