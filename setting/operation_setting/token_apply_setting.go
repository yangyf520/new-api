package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type TokenApplySetting struct {
	ApiKey       string `json:"api_key"`
	AppUserEmail string `json:"app_user_email"`
}

var tokenApplySetting = TokenApplySetting{
	ApiKey: "",
}

func init() {
	config.GlobalConfig.Register("token_apply_setting", &tokenApplySetting)
}

func GetTokenApplySetting() *TokenApplySetting {
	return &tokenApplySetting
}

func TokenApplyEnabled() bool {
	return strings.TrimSpace(EffectiveTokenApplyApiKey()) != ""
}

func EffectiveTokenApplyApiKey() string {
	if key := strings.TrimSpace(common.GetEnvOrDefaultString("TOKEN_API_KEY", "")); key != "" {
		return key
	}
	return strings.TrimSpace(tokenApplySetting.ApiKey)
}
