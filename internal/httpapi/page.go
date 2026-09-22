package httpapi

import (
	"strconv"
	"sudoStream/internal/mediafs"

	"github.com/gin-gonic/gin"
)

func parsePageOpts(c *gin.Context) mediafs.PageOpts {
	return mediafs.NormalizePageOpts(parseRawPageOpts(c))
}

// parseRawPageOpts reads limit/offset without applying defaults (limit 0 = unset).
func parseRawPageOpts(c *gin.Context) mediafs.PageOpts {
	opts := mediafs.PageOpts{}
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil {
			opts.Limit = parsed
		}
	}
	if raw := c.Query("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil {
			opts.Offset = parsed
		}
	}

	return opts
}
