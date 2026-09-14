package controller

import (
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"github.com/gin-gonic/gin"
	"strings"
)

// AIRecognitionController 提供AI配置、模型发现及连接试验接口。
type AIRecognitionController struct{ service *service.AIRecognitionService }

// NewAIRecognitionController 注入可测试的AI配置服务。
func NewAIRecognitionController(s *service.AIRecognitionService) *AIRecognitionController {
	return &AIRecognitionController{service: s}
}

// Get 返回配置，已保存的密钥仅返回存在标记。
func (c *AIRecognitionController) Get(x *gin.Context) {
	value, err := c.service.PublicConfig()
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, value)
}

// Save 原子保存AI配置，后续识别立即生效。
func (c *AIRecognitionController) Save(x *gin.Context) {
	var value domain.AIRecognitionConfig
	if x.ShouldBindJSON(&value) != nil {
		ErrorResp(x, 400, "AI配置格式无效")
		return
	}
	if err := c.service.Save(value); err != nil {
		ErrorResp(x, 400, err.Error())
		return
	}
	c.Get(x)
}

// Models 返回当前表单端点声明支持的模型ID，不能保证每个模型都支持聊天。
func (c *AIRecognitionController) Models(x *gin.Context) {
	var value domain.AIRecognitionConfig
	if x.ShouldBindJSON(&value) != nil {
		ErrorResp(x, 400, "AI配置格式无效")
		return
	}
	models, err := c.service.Models(x, value)
	if err != nil {
		ErrorResp(x, 502, err.Error())
		return
	}
	SuccessResp(x, models)
}

// Test 用用户提供的文件名测试聊天协议与提示词，不保存配置或媒体结果。
func (c *AIRecognitionController) Test(x *gin.Context) {
	var input struct {
		Config   domain.AIRecognitionConfig `json:"config"`
		Filename string                     `json:"filename"`
	}
	if x.ShouldBindJSON(&input) != nil {
		ErrorResp(x, 400, "测试参数无效")
		return
	}
	hint, err := c.service.Test(x, input.Config, input.Filename)
	if err != nil {
		ErrorResp(x, 502, err.Error())
		return
	}
	SuccessResp(x, hint)
}

// Assist 使用已保存配置解析文件名，只返回需要用户继续核验的查询建议。
func (c *AIRecognitionController) Assist(x *gin.Context) {
	var input struct {
		Filename string `json:"filename"`
	}
	if x.ShouldBindJSON(&input) != nil {
		ErrorResp(x, 400, "AI解析参数无效")
		return
	}
	if strings.TrimSpace(input.Filename) == "" {
		ErrorResp(x, 400, "请输入要解析的文件名")
		return
	}
	hint, err := c.service.AssistManual(x.Request.Context(), input.Filename)
	if err != nil {
		ErrorResp(x, 502, err.Error())
		return
	}
	SuccessResp(x, hint)
}
