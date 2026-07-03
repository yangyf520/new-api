package service

import (
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	LogRecordVersion   = 1
	LogRecordMaxRunes  = 32000
	MediaRefOmitted    = "[omitted]"
	MediaRefPrefixURL  = "url:"
	MediaRefPrefixFile = "file:"
)

const (
	LogKindChat        = "chat"
	LogKindCompletion  = "completion"
	LogKindEmbedding   = "embedding"
	LogKindModeration  = "moderation"
	LogKindRerank      = "rerank"
	LogKindImage       = "image"
	LogKindAudio       = "audio"
	LogKindCustom      = "custom"
)

const (
	LogItemTypeText       = "text"
	LogItemTypeImage      = "image"
	LogItemTypeAudio      = "audio"
	LogItemTypeFile       = "file"
	LogItemTypeVideo      = "video"
	LogItemTypeToolCall   = "tool_call"
	LogItemTypeToolResult = "tool_result"
	LogItemTypeParam      = "param"
)

type LogRecord struct {
	V     int       `json:"v"`
	Kind  string    `json:"kind"`
	Items []LogItem `json:"items"`
}

type LogItem struct {
	Role      string `json:"role,omitempty"`
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Ref       string `json:"ref,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Name      string `json:"name,omitempty"`
	ID        string `json:"id,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Key       string `json:"key,omitempty"`
	Value     string `json:"value,omitempty"`
}

func BuildLogInput(c *gin.Context, info *relaycommon.RelayInfo) LogRecord {
	if info == nil {
		return emptyLogRecord(LogKindCustom)
	}
	if info.Request != nil {
		if record, ok := buildLogInputFromRequest(info); ok {
			return record
		}
	}
	if c != nil {
		if record, ok := buildLogInputFromRelayMode(c, info); ok {
			return record
		}
	}
	return emptyLogRecord(inferLogKind(info))
}

func BuildLogOutput(info *relaycommon.RelayInfo, raw string) LogRecord {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return emptyLogRecord(outputLogKind(info))
	}
	return buildLogOutputFromRaw(info, outputLogKind(info), raw)
}

func MarshalLogRecord(record LogRecord) string {
	if len(record.Items) == 0 {
		return ""
	}
	if record.V == 0 {
		record.V = LogRecordVersion
	}
	data, err := common.Marshal(record)
	if err != nil {
		return ""
	}
	return string(data)
}

func emptyLogRecord(kind string) LogRecord {
	if kind == "" {
		kind = LogKindCustom
	}
	return LogRecord{V: LogRecordVersion, Kind: kind, Items: nil}
}

func newLogRecord(kind string, items []LogItem) LogRecord {
	if kind == "" {
		kind = LogKindCustom
	}
	return LogRecord{V: LogRecordVersion, Kind: kind, Items: items}
}

func truncateLogText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if utf8.RuneCountInString(text) <= LogRecordMaxRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:LogRecordMaxRunes]) + "...[truncated]"
}

func appendTextItem(items *[]LogItem, role, text string) {
	text = truncateLogText(text)
	if text == "" {
		return
	}
	*items = append(*items, LogItem{
		Role: role,
		Type: LogItemTypeText,
		Text: text,
	})
}

func appendParam(items *[]LogItem, key, value string) {
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	*items = append(*items, LogItem{
		Type:  LogItemTypeParam,
		Key:   key,
		Value: truncateLogText(value),
	})
}

func appendMediaRef(items *[]LogItem, role, itemType, ref, mime, name string) {
	if ref == "" {
		ref = MediaRefOmitted
	}
	item := LogItem{
		Role: role,
		Type: itemType,
		Ref:  ref,
		Mime: mime,
		Name: name,
	}
	*items = append(*items, item)
}

func isLikelyInlineMediaData(data string) bool {
	data = strings.TrimSpace(data)
	if data == "" {
		return false
	}
	if strings.HasPrefix(data, "data:") {
		return true
	}
	if strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://") {
		return false
	}
	return len(data) > 256
}

func mediaRefFromRawData(data, mime string) (ref, resolvedMime string) {
	data = strings.TrimSpace(data)
	if data == "" {
		return "", mime
	}
	if strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://") {
		return MediaRefPrefixURL + data, mime
	}
	if isLikelyInlineMediaData(data) {
		return MediaRefOmitted, mime
	}
	return MediaRefPrefixFile + truncateLogText(data), mime
}

func inferLogKind(info *relaycommon.RelayInfo) string {
	if info == nil {
		return LogKindCustom
	}
	if kind := relayModeLogKind(info.RelayMode); kind != LogKindCustom {
		return kind
	}
	return relayFormatLogKind(info.GetFinalRequestRelayFormat())
}

func relayFormatLogKind(format types.RelayFormat) string {
	switch format {
	case types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini,
		types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction,
		types.RelayFormatOpenAIRealtime:
		return LogKindChat
	case types.RelayFormatEmbedding:
		return LogKindEmbedding
	case types.RelayFormatOpenAIImage:
		return LogKindImage
	case types.RelayFormatOpenAIAudio:
		return LogKindAudio
	case types.RelayFormatRerank:
		return LogKindRerank
	default:
		return LogKindCustom
	}
}

func outputLogKind(info *relaycommon.RelayInfo) string {
	return inferLogKind(info)
}
