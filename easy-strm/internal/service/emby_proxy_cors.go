package service

import (
	"net/http"
	"strings"
)

func applyProxyCORS(header http.Header, request *http.Request) {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return
	}
	header.Set("Access-Control-Allow-Origin", origin)
	header.Set("Access-Control-Allow-Credentials", "true")
	header.Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges, Content-Type")
	for _, line := range header.Values("Vary") {
		for _, value := range strings.Split(line, ",") {
			if strings.EqualFold(strings.TrimSpace(value), "Origin") {
				return
			}
		}
	}
	header.Add("Vary", "Origin")
}
