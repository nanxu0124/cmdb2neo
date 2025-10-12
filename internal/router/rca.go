package router

import (
	"fmt"
	"strings"
	"time"

	rca "cmdb2neo/internal/rca"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RCAHandler 负责处理根因分析相关的 HTTP 请求。
type RCAHandler struct {
	analyzer *rca.Analyzer
	logger   *zap.Logger
}

// NewRCAHandler 构建一个新的 RCAHandler。
func NewRCAHandler(analyzer *rca.Analyzer, logger *zap.Logger) *RCAHandler {
	return &RCAHandler{analyzer: analyzer, logger: logger}
}

// RegisterRoutes 将根因分析路由注册到给定的路由组。
func (h *RCAHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/analyze", h.handleAnalyze)
}

type analyzeRequest struct {
	WindowID string           `json:"window_id"`
	Events   []rca.AlarmEvent `json:"events"`
}

type analyzeResponse struct {
	WindowID string     `json:"window_id"`
	Result   rca.Result `json:"result"`
}

func (h *RCAHandler) handleAnalyze(c *gin.Context) {
	var req analyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request payload"})
		return
	}
	if len(req.Events) == 0 {
		c.JSON(400, gin.H{"error": "events payload is empty"})
		return
	}
	windowID := strings.TrimSpace(req.WindowID)
	if windowID == "" {
		windowID = fmt.Sprintf("auto-%d", time.Now().Unix())
	}
	result, err := h.analyzer.Analyze(c.Request.Context(), req.Events)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("analyze failed", zap.Error(err))
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, analyzeResponse{WindowID: windowID, Result: result})
}
