package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func buildLogInputFromRequest(info *relaycommon.RelayInfo) (LogRecord, bool) {
	switch req := info.Request.(type) {
	case *dto.GeneralOpenAIRequest:
		if info.RelayMode == relayconstant.RelayModeCompletions || info.RelayMode == relayconstant.RelayModeEdits {
			return extractCompletionInput(req), true
		}
		return extractChatInputFromOpenAI(req), true
	case *dto.ClaudeRequest:
		return extractChatInputFromClaude(req), true
	case *dto.GeminiChatRequest:
		return extractChatInputFromGemini(req), true
	case *dto.GeminiEmbeddingRequest:
		return extractGeminiEmbeddingInput(req), true
	case *dto.GeminiBatchEmbeddingRequest:
		return extractGeminiBatchEmbeddingInput(req), true
	case *dto.EmbeddingRequest:
		kind := LogKindEmbedding
		if info.RelayMode == relayconstant.RelayModeModerations {
			kind = LogKindModeration
		}
		return extractEmbeddingInput(req, kind), true
	case *dto.RerankRequest:
		return extractRerankInput(req), true
	case *dto.ImageRequest:
		return extractImageInput(req), true
	case *dto.AudioRequest:
		return extractAudioInput(req, info.RelayMode), true
	case *dto.OpenAIResponsesRequest:
		return extractChatInputFromResponses(req), true
	case *dto.OpenAIResponsesCompactionRequest:
		return extractResponsesCompactionInput(req), true
	default:
		return LogRecord{}, false
	}
}

func buildLogInputFromRelayMode(c *gin.Context, info *relaycommon.RelayInfo) (LogRecord, bool) {
	switch info.RelayMode {
	case relayconstant.RelayModeMidjourneyImagine,
		relayconstant.RelayModeMidjourneyDescribe,
		relayconstant.RelayModeMidjourneyBlend,
		relayconstant.RelayModeMidjourneyChange,
		relayconstant.RelayModeMidjourneySimpleChange,
		relayconstant.RelayModeMidjourneyAction,
		relayconstant.RelayModeMidjourneyModal,
		relayconstant.RelayModeMidjourneyShorten,
		relayconstant.RelayModeMidjourneyUpload,
		relayconstant.RelayModeMidjourneyVideo,
		relayconstant.RelayModeMidjourneyEdits:
		var req taskdto.MidjourneyRequest
		if err := common.UnmarshalBodyReusable(c, &req); err != nil {
			return LogRecord{}, false
		}
		return BuildLogInputFromMidjourney(&req, info.RelayMode), true
	case relayconstant.RelayModeSwapFace:
		var req taskdto.SwapFaceRequest
		if err := common.UnmarshalBodyReusable(c, &req); err != nil {
			return LogRecord{}, false
		}
		return BuildLogInputFromSwapFace(&req), true
	default:
		return LogRecord{}, false
	}
}

func BuildLogInputFromMidjourney(req *taskdto.MidjourneyRequest, relayMode int) LogRecord {
	if req == nil {
		return emptyLogRecord(LogKindCustom)
	}
	var items []LogItem
	appendTextItem(&items, "user", req.Prompt)
	if req.Content != "" {
		appendTextItem(&items, "user", req.Content)
	}
	appendParam(&items, "action", firstNonEmpty(req.Action, relayModeActionLabel(relayMode)))
	if req.TaskId != "" {
		appendParam(&items, "task_id", req.TaskId)
	}
	if req.CustomId != "" {
		appendParam(&items, "custom_id", req.CustomId)
	}
	if req.Index > 0 {
		appendParam(&items, "index", fmt.Sprintf("%d", req.Index))
	}
	if len(req.Base64Array) > 0 {
		appendMediaRef(&items, "user", LogItemTypeImage, MediaRefOmitted, "", "base64_array")
	}
	if req.MaskBase64 != "" {
		appendMediaRef(&items, "user", LogItemTypeImage, MediaRefOmitted, "", "mask")
	}
	return newLogRecord(LogKindCustom, items)
}

func BuildLogInputFromSwapFace(req *taskdto.SwapFaceRequest) LogRecord {
	if req == nil {
		return emptyLogRecord(LogKindCustom)
	}
	var items []LogItem
	appendParam(&items, "action", "swap_face")
	if req.SourceBase64 != "" {
		appendMediaRef(&items, "user", LogItemTypeImage, MediaRefOmitted, "", "source")
	}
	if req.TargetBase64 != "" {
		appendMediaRef(&items, "user", LogItemTypeImage, MediaRefOmitted, "", "target")
	}
	return newLogRecord(LogKindCustom, items)
}

func extractChatInputFromOpenAI(req *dto.GeneralOpenAIRequest) LogRecord {
	var items []LogItem
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		if message.IsStringContent() {
			appendTextItem(&items, role, message.StringContent())
			continue
		}
		for _, part := range message.ParseContent() {
			appendOpenAIMediaContent(&items, role, part)
		}
		for _, toolCall := range message.ParseToolCalls() {
			appendToolCallItem(&items, role, toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments)
		}
	}
	return newLogRecord(LogKindChat, items)
}

func appendOpenAIMediaContent(items *[]LogItem, role string, part dto.MediaContent) {
	switch part.Type {
	case dto.ContentTypeText:
		appendTextItem(items, role, part.Text)
	case dto.ContentTypeImageURL:
		appendFileSourceItem(items, role, LogItemTypeImage, part.ToFileSource())
	case dto.ContentTypeInputAudio:
		appendFileSourceItem(items, role, LogItemTypeAudio, part.ToFileSource())
	case dto.ContentTypeFile:
		appendFileSourceItem(items, role, LogItemTypeFile, part.ToFileSource())
	case dto.ContentTypeVideoUrl:
		appendFileSourceItem(items, role, LogItemTypeVideo, part.ToFileSource())
	default:
		if part.Text != "" {
			appendTextItem(items, role, part.Text)
		}
	}
}

func appendFileSourceItem(items *[]LogItem, role, itemType string, source types.FileSource) {
	if source == nil {
		return
	}
	ref, mime := mediaRefFromFileSource(source)
	appendMediaRef(items, role, itemType, ref, mime, "")
}

func mediaRefFromFileSource(source types.FileSource) (ref, mime string) {
	if source == nil {
		return MediaRefOmitted, ""
	}
	if source.IsURL() {
		return MediaRefPrefixURL + source.GetRawData(), ""
	}
	if base64Source, ok := source.(*types.Base64Source); ok {
		return MediaRefOmitted, base64Source.MimeType
	}
	return MediaRefOmitted, ""
}

func appendToolCallItem(items *[]LogItem, role, id, name, arguments string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	*items = append(*items, LogItem{
		Role:      role,
		Type:      LogItemTypeToolCall,
		ID:        id,
		Name:      name,
		Arguments: truncateLogText(arguments),
	})
}

func extractCompletionInput(req *dto.GeneralOpenAIRequest) LogRecord {
	var items []LogItem
	appendTextItem(&items, "user", promptFromAny(req.Prompt))
	if req.Instruction != "" {
		appendTextItem(&items, "system", req.Instruction)
	}
	return newLogRecord(LogKindCompletion, items)
}

func promptFromAny(prompt any) string {
	switch value := prompt.(type) {
	case nil:
		return ""
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprintf("%v", value)
	}
}

func extractChatInputFromClaude(req *dto.ClaudeRequest) LogRecord {
	var items []LogItem
	if req.System != nil {
		if req.IsStringSystem() {
			appendTextItem(&items, "system", req.GetStringSystem())
		} else {
			for _, part := range req.ParseSystem() {
				appendClaudeMedia(&items, "system", part)
			}
		}
	}
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		if message.IsStringContent() {
			appendTextItem(&items, role, message.GetStringContent())
			continue
		}
		parts, err := message.ParseContent()
		if err != nil {
			continue
		}
		for _, part := range parts {
			appendClaudeMedia(&items, role, part)
		}
	}
	return newLogRecord(LogKindChat, items)
}

func appendClaudeMedia(items *[]LogItem, role string, part dto.ClaudeMediaMessage) {
	switch part.Type {
	case "text", "input_text":
		appendTextItem(items, role, part.GetText())
	case "image":
		appendFileSourceItem(items, role, LogItemTypeImage, part.ToFileSource())
	case "tool_use":
		args, _ := common.Marshal(part.Input)
		appendToolCallItem(items, role, part.Id, part.Name, string(args))
	case "tool_result":
		appendTextItem(items, "tool", toolResultText(part))
	case "thinking":
		if part.Thinking != nil && *part.Thinking != "" {
			appendTextItem(items, role, *part.Thinking)
		}
	default:
		if text := part.GetText(); text != "" {
			appendTextItem(items, role, text)
		} else if text := part.GetStringContent(); text != "" {
			appendTextItem(items, role, text)
		}
	}
}

func toolResultText(part dto.ClaudeMediaMessage) string {
	if part.IsStringContent() {
		return part.GetStringContent()
	}
	if part.Content == nil {
		return ""
	}
	data, err := common.Marshal(part.Content)
	if err != nil {
		return fmt.Sprintf("%v", part.Content)
	}
	return string(data)
}

func extractChatInputFromGemini(req *dto.GeminiChatRequest) LogRecord {
	var items []LogItem
	if req.SystemInstructions != nil {
		for _, part := range req.SystemInstructions.Parts {
			appendGeminiPart(&items, "system", part)
		}
	}
	for _, content := range req.Contents {
		role := normalizeGeminiRole(content.Role)
		for _, part := range content.Parts {
			appendGeminiPart(&items, role, part)
		}
	}
	return newLogRecord(LogKindChat, items)
}

func normalizeGeminiRole(role string) string {
	role = strings.TrimSpace(role)
	switch role {
	case "", "user":
		return "user"
	case "model":
		return "assistant"
	default:
		return role
	}
}

func appendGeminiPart(items *[]LogItem, role string, part dto.GeminiPart) {
	if part.Text != "" {
		appendTextItem(items, role, part.Text)
	}
	if part.InlineData != nil {
		appendFileSourceItem(items, role, LogItemTypeImage, part.InlineData.ToFileSource())
	}
	if part.FileData != nil && part.FileData.FileUri != "" {
		ref, mime := mediaRefFromRawData(part.FileData.FileUri, part.FileData.MimeType)
		appendMediaRef(items, role, LogItemTypeFile, ref, mime, "")
	}
	if part.FunctionCall != nil {
		args, _ := common.Marshal(part.FunctionCall.Arguments)
		appendToolCallItem(items, role, "", part.FunctionCall.FunctionName, string(args))
	}
	if part.FunctionResponse != nil {
		data, _ := common.Marshal(part.FunctionResponse.Response)
		*items = append(*items, LogItem{
			Role: role,
			Type: LogItemTypeToolResult,
			Name: part.FunctionResponse.Name,
			Text: truncateLogText(string(data)),
		})
	}
	if part.CodeExecutionResult != nil && part.CodeExecutionResult.Output != "" {
		appendTextItem(items, role, part.CodeExecutionResult.Output)
	}
}

func extractGeminiEmbeddingInput(req *dto.GeminiEmbeddingRequest) LogRecord {
	var items []LogItem
	for _, part := range req.Content.Parts {
		appendTextItem(&items, "", part.Text)
	}
	return newLogRecord(LogKindEmbedding, items)
}

func extractGeminiBatchEmbeddingInput(req *dto.GeminiBatchEmbeddingRequest) LogRecord {
	var items []LogItem
	for _, request := range req.Requests {
		if request == nil {
			continue
		}
		record := extractGeminiEmbeddingInput(request)
		items = append(items, record.Items...)
	}
	return newLogRecord(LogKindEmbedding, items)
}

func extractEmbeddingInput(req *dto.EmbeddingRequest, kind string) LogRecord {
	var items []LogItem
	for _, text := range req.ParseInput() {
		appendTextItem(&items, "", text)
	}
	return newLogRecord(kind, items)
}

func extractRerankInput(req *dto.RerankRequest) LogRecord {
	var items []LogItem
	appendTextItem(&items, "query", req.Query)
	for _, document := range req.Documents {
		appendTextItem(&items, "document", fmt.Sprintf("%v", document))
	}
	return newLogRecord(LogKindRerank, items)
}

func extractImageInput(req *dto.ImageRequest) LogRecord {
	var items []LogItem
	appendTextItem(&items, "", req.Prompt)
	appendParam(&items, "model", req.Model)
	appendParam(&items, "size", req.Size)
	appendParam(&items, "quality", req.Quality)
	if req.N != nil {
		appendParam(&items, "n", fmt.Sprintf("%d", *req.N))
	}
	if req.IsStream(nil) {
		appendParam(&items, "stream", "true")
	}
	appendRawMediaField(&items, LogItemTypeImage, "image", req.Image)
	appendRawMediaField(&items, LogItemTypeImage, "images", req.Images)
	appendRawMediaField(&items, LogItemTypeImage, "mask", req.Mask)
	return newLogRecord(LogKindImage, items)
}

func appendRawMediaField(items *[]LogItem, itemType, name string, raw []byte) {
	if len(raw) == 0 {
		return
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return
	}
	if isLikelyInlineMediaData(text) {
		appendMediaRef(items, "user", itemType, MediaRefOmitted, "", name)
		return
	}
	ref, mime := mediaRefFromRawData(text, "")
	appendMediaRef(items, "user", itemType, ref, mime, name)
}

func extractAudioInput(req *dto.AudioRequest, relayMode int) LogRecord {
	var items []LogItem
	switch relayMode {
	case relayconstant.RelayModeAudioSpeech:
		appendTextItem(&items, "", req.Input)
	default:
		appendMediaRef(&items, "user", LogItemTypeAudio, MediaRefOmitted, "", "file")
	}
	appendParam(&items, "model", req.Model)
	appendParam(&items, "voice", req.Voice)
	appendParam(&items, "response_format", req.ResponseFormat)
	appendParam(&items, "language", strings.TrimSpace(string(req.Language)))
	return newLogRecord(LogKindAudio, items)
}

func extractChatInputFromResponses(req *dto.OpenAIResponsesRequest) LogRecord {
	var items []LogItem
	if len(req.Instructions) > 0 {
		appendTextItem(&items, "system", string(req.Instructions))
	}
	for _, input := range req.ParseInput() {
		role := strings.TrimSpace(input.Type)
		if role == "input_text" || role == "" {
			role = "user"
		}
		switch input.Type {
		case "input_text":
			appendTextItem(&items, "user", input.Text)
		case "input_image":
			ref, _ := mediaRefFromRawData(input.ImageUrl, "")
			appendMediaRef(&items, "user", LogItemTypeImage, ref, input.Detail, "")
		case "input_file":
			ref, _ := mediaRefFromRawData(input.FileUrl, "")
			appendMediaRef(&items, "user", LogItemTypeFile, ref, "", "")
		default:
			if input.Text != "" {
				appendTextItem(&items, role, input.Text)
			}
		}
	}
	return newLogRecord(LogKindChat, items)
}

func extractResponsesCompactionInput(req *dto.OpenAIResponsesCompactionRequest) LogRecord {
	var items []LogItem
	if len(req.Instructions) > 0 {
		appendTextItem(&items, "system", string(req.Instructions))
	}
	if len(req.Input) > 0 {
		appendTextItem(&items, "user", string(req.Input))
	}
	if req.PreviousResponseID != "" {
		appendParam(&items, "previous_response_id", req.PreviousResponseID)
	}
	return newLogRecord(LogKindChat, items)
}

func relayModeActionLabel(relayMode int) string {
	switch relayMode {
	case relayconstant.RelayModeMidjourneyImagine:
		return "imagine"
	case relayconstant.RelayModeMidjourneyDescribe:
		return "describe"
	case relayconstant.RelayModeMidjourneyBlend:
		return "blend"
	case relayconstant.RelayModeMidjourneyShorten:
		return "shorten"
	case relayconstant.RelayModeMidjourneyUpload:
		return "upload"
	case relayconstant.RelayModeMidjourneyVideo:
		return "video"
	case relayconstant.RelayModeMidjourneyEdits:
		return "edits"
	default:
		return ""
	}
}

func relayModeLogKind(relayMode int) string {
	switch relayMode {
	case relayconstant.RelayModeCompletions, relayconstant.RelayModeEdits:
		return LogKindCompletion
	case relayconstant.RelayModeEmbeddings:
		return LogKindEmbedding
	case relayconstant.RelayModeModerations:
		return LogKindModeration
	case relayconstant.RelayModeRerank:
		return LogKindRerank
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		return LogKindImage
	case relayconstant.RelayModeAudioSpeech, relayconstant.RelayModeAudioTranscription, relayconstant.RelayModeAudioTranslation:
		return LogKindAudio
	case relayconstant.RelayModeChatCompletions, relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact,
		relayconstant.RelayModeGemini, relayconstant.RelayModeRealtime:
		return LogKindChat
	default:
		return LogKindCustom
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
