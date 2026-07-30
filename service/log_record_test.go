package service

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func TestBuildLogInputChat(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
		},
	}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeChatCompletions,
		Request:   req,
	}

	record := BuildLogInput(nil, info)
	if record.Kind != LogKindChat {
		t.Fatalf("expected kind chat, got %s", record.Kind)
	}
	if len(record.Items) != 1 || record.Items[0].Text != "hello" {
		t.Fatalf("unexpected items: %+v", record.Items)
	}

	output := MarshalLogRecord(record)
	if output == "" {
		t.Fatal("expected non-empty marshaled record")
	}
}

func TestBuildLogInputEmbedding(t *testing.T) {
	req := &dto.EmbeddingRequest{
		Input: []any{"first", "second"},
	}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeEmbeddings,
		Request:   req,
	}

	record := BuildLogInput(nil, info)
	if record.Kind != LogKindEmbedding {
		t.Fatalf("expected kind embedding, got %s", record.Kind)
	}
	if len(record.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(record.Items))
	}
}

func TestBuildLogInputImageOmitsInlineData(t *testing.T) {
	req := &dto.ImageRequest{
		Prompt: "draw a cat",
		Size:   "1024x1024",
		Image:  []byte(`"data:image/png;base64,` + string(make([]byte, 300)) + `"`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		Request:   req,
	}

	record := BuildLogInput(nil, info)
	if record.Kind != LogKindImage {
		t.Fatalf("expected kind image, got %s", record.Kind)
	}
	foundOmitted := false
	for _, item := range record.Items {
		if item.Ref == MediaRefOmitted {
			foundOmitted = true
		}
		if item.Text != "" && len(item.Text) > 300 {
			t.Fatalf("inline image data should not be stored as text")
		}
	}
	if !foundOmitted {
		t.Fatalf("expected omitted media ref, got %+v", record.Items)
	}
}

func TestBuildLogOutputEmbeddingEmpty(t *testing.T) {
	raw := `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}]}`
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeEmbeddings}
	record := BuildLogOutput(info, raw)
	if MarshalLogRecord(record) != "" {
		t.Fatalf("embedding output should be empty, got %+v", record)
	}
}

func TestBuildLogOutputChat(t *testing.T) {
	raw := `{"choices":[{"message":{"role":"assistant","content":"hi there"}}]}`
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions}
	record := BuildLogOutput(info, raw)
	if len(record.Items) != 1 || record.Items[0].Text != "hi there" {
		t.Fatalf("unexpected output items: %+v", record.Items)
	}
}

func TestBuildLogOutputStream(t *testing.T) {
	raw := `{"stream":true,"content":"streamed answer"}`
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions}
	record := BuildLogOutput(info, raw)
	if len(record.Items) != 1 || record.Items[0].Text != "streamed answer" {
		t.Fatalf("unexpected stream output: %+v", record.Items)
	}
}

func TestBuildLogOutputClaude(t *testing.T) {
	raw := `{"id":"msg_01","type":"message","role":"assistant","content":[{"type":"text","text":"Bonjour"}]}`
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeChatCompletions,
		RelayFormat: types.RelayFormatClaude,
	}
	record := BuildLogOutput(info, raw)
	if len(record.Items) != 1 || record.Items[0].Text != "Bonjour" {
		t.Fatalf("unexpected claude output: %+v", record.Items)
	}
}

func TestBuildLogOutputClaudeThinking(t *testing.T) {
	raw := `{"type":"message","role":"assistant","content":[{"type":"thinking","thinking":"let me think"},{"type":"text","text":"answer"}]}`
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}
	record := BuildLogOutput(info, raw)
	if len(record.Items) != 2 {
		t.Fatalf("expected thinking + text, got %+v", record.Items)
	}
	if record.Items[0].Text != "let me think" || record.Items[1].Text != "answer" {
		t.Fatalf("unexpected claude thinking output: %+v", record.Items)
	}
}

func TestBuildLogOutputGemini(t *testing.T) {
	raw := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello from Gemini"}]}}]}`
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeGemini,
		RelayFormat: types.RelayFormatGemini,
	}
	record := BuildLogOutput(info, raw)
	if len(record.Items) != 1 || record.Items[0].Text != "Hello from Gemini" {
		t.Fatalf("unexpected gemini output: %+v", record.Items)
	}
}

func TestBuildLogOutputGeminiToolCall(t *testing.T) {
	raw := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Paris"}}}]}}]}`
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatGemini}
	record := BuildLogOutput(info, raw)
	if len(record.Items) != 1 || record.Items[0].Type != LogItemTypeToolCall || record.Items[0].Name != "get_weather" {
		t.Fatalf("unexpected gemini tool output: %+v", record.Items)
	}
}

func TestBuildLogInputClaude(t *testing.T) {
	req := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "explain moon"},
		},
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		Request:     req,
	}
	record := BuildLogInput(nil, info)
	if len(record.Items) != 1 || record.Items[0].Text != "explain moon" {
		t.Fatalf("unexpected claude input: %+v", record.Items)
	}
}

func TestBuildLogInputGemini(t *testing.T) {
	req := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{Role: "user", Parts: []dto.GeminiPart{{Text: "hi gemini"}}},
		},
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatGemini,
		Request:     req,
	}
	record := BuildLogInput(nil, info)
	if len(record.Items) != 1 || record.Items[0].Text != "hi gemini" {
		t.Fatalf("unexpected gemini input: %+v", record.Items)
	}
}

func TestBuildLogInputFromMidjourney(t *testing.T) {
	req := &dto.MidjourneyRequest{
		Prompt:      "sunset",
		Base64Array: []string{"abc"},
	}
	record := BuildLogInputFromMidjourney(req, relayconstant.RelayModeMidjourneyImagine)
	if record.Items[0].Text != "sunset" {
		t.Fatalf("unexpected prompt item: %+v", record.Items[0])
	}
	foundOmitted := false
	for _, item := range record.Items {
		if item.Ref == MediaRefOmitted {
			foundOmitted = true
		}
	}
	if !foundOmitted {
		t.Fatal("expected omitted base64 media ref")
	}
}
