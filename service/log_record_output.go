package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

func buildLogOutputFromRaw(info *relaycommon.RelayInfo, kind, raw string) LogRecord {
	if streamRecord, ok := parseStreamModelOutput(raw); ok {
		return streamRecord
	}

	switch kind {
	case LogKindEmbedding, LogKindModeration:
		return emptyLogRecord(kind)
	case LogKindImage:
		return extractImageOutput(raw)
	case LogKindAudio:
		return extractAudioOutput(raw)
	case LogKindRerank:
		return extractRerankOutput(raw)
	case LogKindCompletion:
		if record, ok := extractCompletionOutput(raw); ok {
			return record
		}
	case LogKindChat:
		if record, ok := tryExtractChatOutput(info, raw); ok {
			return record
		}
	}

	if record, ok := extractGenericTextOutput(kind, raw); ok {
		return record
	}
	return emptyLogRecord(kind)
}

func tryExtractChatOutput(info *relaycommon.RelayInfo, raw string) (LogRecord, bool) {
	format := types.RelayFormat("")
	if info != nil {
		format = info.GetFinalRequestRelayFormat()
	}

	try := func(extractors ...func(string) (LogRecord, bool)) (LogRecord, bool) {
		for _, extract := range extractors {
			if record, ok := extract(raw); ok {
				return record, true
			}
		}
		return LogRecord{}, false
	}

	switch format {
	case types.RelayFormatClaude:
		return try(extractClaudeOutput, extractChatOutput, extractResponsesOutput, extractGeminiChatOutput)
	case types.RelayFormatGemini:
		return try(extractGeminiChatOutput, extractChatOutput, extractClaudeOutput, extractResponsesOutput)
	default:
		return try(extractChatOutput, extractResponsesOutput, extractClaudeOutput, extractGeminiChatOutput)
	}
}

func extractClaudeOutput(raw string) (LogRecord, bool) {
	var response dto.ClaudeResponse
	if err := common.UnmarshalJsonStr(raw, &response); err != nil {
		return LogRecord{}, false
	}

	var items []LogItem
	role := strings.TrimSpace(response.Role)
	if role == "" {
		role = "assistant"
	}

	if strings.TrimSpace(response.Completion) != "" {
		appendTextItem(&items, role, response.Completion)
	}
	for _, block := range response.Content {
		appendClaudeMedia(&items, role, block)
	}
	if response.Message != nil {
		msgRole := strings.TrimSpace(response.Message.Role)
		if msgRole == "" {
			msgRole = role
		}
		if response.Message.IsStringContent() {
			appendTextItem(&items, msgRole, response.Message.GetStringContent())
		} else {
			for _, block := range response.Message.ParseMediaContent() {
				appendClaudeMedia(&items, msgRole, block)
			}
		}
	}
	if response.ContentBlock != nil {
		appendClaudeMedia(&items, role, *response.ContentBlock)
	}

	if len(items) == 0 {
		return LogRecord{}, false
	}
	return newLogRecord(LogKindChat, items), true
}

func extractGeminiChatOutput(raw string) (LogRecord, bool) {
	var response dto.GeminiChatResponse
	if err := common.UnmarshalJsonStr(raw, &response); err != nil {
		return LogRecord{}, false
	}
	if len(response.Candidates) == 0 {
		return LogRecord{}, false
	}

	var items []LogItem
	for _, candidate := range response.Candidates {
		role := normalizeGeminiRole(candidate.Content.Role)
		if role == "" || role == "user" {
			role = "assistant"
		}
		for _, part := range candidate.Content.Parts {
			appendGeminiPart(&items, role, part)
		}
	}
	if len(items) == 0 {
		return LogRecord{}, false
	}
	return newLogRecord(LogKindChat, items), true
}

func extractGeminiImageOutput(raw string) LogRecord {
	var response dto.GeminiImageResponse
	if err := common.UnmarshalJsonStr(raw, &response); err != nil {
		return emptyLogRecord(LogKindImage)
	}
	var items []LogItem
	for _, prediction := range response.Predictions {
		if prediction.RaiFilteredReason != "" {
			appendParam(&items, "filtered_reason", prediction.RaiFilteredReason)
		}
		if prediction.BytesBase64Encoded != "" {
			appendMediaRef(&items, "assistant", LogItemTypeImage, MediaRefOmitted, prediction.MimeType, "")
		}
	}
	return newLogRecord(LogKindImage, items)
}

func parseStreamModelOutput(raw string) (LogRecord, bool) {
	var stream logStreamModelOutput
	if err := common.UnmarshalJsonStr(raw, &stream); err != nil {
		return LogRecord{}, false
	}
	if !stream.Stream || strings.TrimSpace(stream.Content) == "" {
		return LogRecord{}, false
	}
	var items []LogItem
	appendTextItem(&items, "assistant", stream.Content)
	return newLogRecord(LogKindChat, items), true
}

func extractChatOutput(raw string) (LogRecord, bool) {
	var response dto.OpenAITextResponse
	if err := common.UnmarshalJsonStr(raw, &response); err != nil || len(response.Choices) == 0 {
		return LogRecord{}, false
	}
	var items []LogItem
	for _, choice := range response.Choices {
		role := strings.TrimSpace(choice.Message.Role)
		if role == "" {
			role = "assistant"
		}
		if choice.Message.IsStringContent() {
			appendTextItem(&items, role, choice.Message.StringContent())
			continue
		}
		for _, part := range choice.Message.ParseContent() {
			appendOpenAIMediaContent(&items, role, part)
		}
		for _, toolCall := range choice.Message.ParseToolCalls() {
			appendToolCallItem(&items, role, toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments)
		}
	}
	if len(items) == 0 {
		return LogRecord{}, false
	}
	return newLogRecord(LogKindChat, items), true
}

func extractCompletionOutput(raw string) (LogRecord, bool) {
	var payload map[string]any
	if err := common.UnmarshalJsonStr(raw, &payload); err != nil {
		return LogRecord{}, false
	}
	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) == 0 {
		return LogRecord{}, false
	}
	var items []LogItem
	for _, choiceAny := range choices {
		choice, ok := choiceAny.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := choice["text"].(string); ok {
			appendTextItem(&items, "assistant", text)
			continue
		}
		message, ok := choice["message"].(map[string]any)
		if !ok {
			continue
		}
		if content, ok := message["content"].(string); ok {
			appendTextItem(&items, "assistant", content)
		}
	}
	if len(items) == 0 {
		return LogRecord{}, false
	}
	return newLogRecord(LogKindCompletion, items), true
}

func extractResponsesOutput(raw string) (LogRecord, bool) {
	var payload map[string]any
	if err := common.UnmarshalJsonStr(raw, &payload); err != nil {
		return LogRecord{}, false
	}
	output, ok := payload["output"].([]any)
	if !ok {
		return LogRecord{}, false
	}
	var items []LogItem
	for _, itemAny := range output {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(asString(item["role"]))
		if role == "" {
			role = "assistant"
		}
		contentParts, ok := item["content"].([]any)
		if !ok {
			continue
		}
		for _, partAny := range contentParts {
			part, ok := partAny.(map[string]any)
			if !ok {
				continue
			}
			switch asString(part["type"]) {
			case "output_text":
				appendTextItem(&items, role, asString(part["text"]))
			case "output_image":
				ref, _ := mediaRefFromRawData(asString(part["image_url"]), "")
				appendMediaRef(&items, role, LogItemTypeImage, ref, "", "")
			default:
				if text := asString(part["text"]); text != "" {
					appendTextItem(&items, role, text)
				}
			}
		}
	}
	if len(items) == 0 {
		return LogRecord{}, false
	}
	return newLogRecord(LogKindChat, items), true
}

func extractImageOutput(raw string) LogRecord {
	var response dto.ImageResponse
	if err := common.UnmarshalJsonStr(raw, &response); err == nil && len(response.Data) > 0 {
		var items []LogItem
		for _, image := range response.Data {
			if image.RevisedPrompt != "" {
				appendTextItem(&items, "assistant", image.RevisedPrompt)
			}
			if image.Url != "" {
				appendMediaRef(&items, "assistant", LogItemTypeImage, MediaRefPrefixURL+image.Url, "", "")
				continue
			}
			if image.B64Json != "" {
				appendMediaRef(&items, "assistant", LogItemTypeImage, MediaRefOmitted, "", "")
			}
		}
		if len(items) > 0 {
			return newLogRecord(LogKindImage, items)
		}
	}
	return extractGeminiImageOutput(raw)
}

func extractAudioOutput(raw string) LogRecord {
	var simple dto.AudioResponse
	if err := common.UnmarshalJsonStr(raw, &simple); err == nil && simple.Text != "" {
		var items []LogItem
		appendTextItem(&items, "assistant", simple.Text)
		return newLogRecord(LogKindAudio, items)
	}
	var verbose dto.WhisperVerboseJSONResponse
	if err := common.UnmarshalJsonStr(raw, &verbose); err == nil && verbose.Text != "" {
		var items []LogItem
		appendTextItem(&items, "assistant", verbose.Text)
		return newLogRecord(LogKindAudio, items)
	}
	return emptyLogRecord(LogKindAudio)
}

func extractRerankOutput(raw string) LogRecord {
	var response dto.RerankResponse
	if err := common.UnmarshalJsonStr(raw, &response); err != nil {
		return emptyLogRecord(LogKindRerank)
	}
	var items []LogItem
	for _, result := range response.Results {
		appendParam(&items, "index", strings.TrimSpace(asString(result.Index)))
		appendParam(&items, "relevance_score", strings.TrimSpace(asString(result.RelevanceScore)))
	}
	return newLogRecord(LogKindRerank, items)
}

func extractGenericTextOutput(kind, raw string) (LogRecord, bool) {
	var payload map[string]any
	if err := common.UnmarshalJsonStr(raw, &payload); err != nil {
		return LogRecord{}, false
	}
	if text := asString(payload["text"]); text != "" {
		var items []LogItem
		appendTextItem(&items, "assistant", text)
		return newLogRecord(kind, items), true
	}
	return LogRecord{}, false
}

func asString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", typed), "0"), ".")
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}
