package controller

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type NetworkController struct {
	runNetworkProbe func(sites interface{}, timeout time.Duration) interface{}
}

func NewNetworkController() *NetworkController {
	return &NetworkController{}
}

func (c *NetworkController) SetRunNetworkProbe(fn func(sites interface{}, timeout time.Duration) interface{}) {
	c.runNetworkProbe = fn
}

func (c *NetworkController) NetworkTest(ctx *gin.Context) {
	if c.runNetworkProbe == nil {
		ErrorResp(ctx, http.StatusInternalServerError, "network probe callback not configured")
		return
	}

	mode := strings.TrimSpace(strings.ToLower(ctx.Query("mode")))
	timeout := 10 * time.Second

	sites := []gin.H{
		{
			"name": "Telegram API",
			"url":  "https://api.telegram.org",
		},
		{
			"name": "Telegram Web",
			"url":  "https://t.me",
		},
		{
			"name": "GitHub",
			"url":  "https://github.com",
		},
		{
			"name": "GitHub API",
			"url":  "https://api.github.com",
		},
		{
			"name": "TMDB API",
			"url":  "https://api.themoviedb.org/3/configuration",
		},
	}

	if mode == "list" {
		SuccessResp(ctx, sites)
		return
	}

	url := strings.TrimSpace(ctx.Query("url"))
	name := strings.TrimSpace(ctx.Query("name"))
	if url != "" {
		if name == "" {
			name = url
		}
		sites = []gin.H{{
			"name": name,
			"url":  url,
		}}
	}

	SuccessResp(ctx, c.runNetworkProbe(sites, timeout))
}
