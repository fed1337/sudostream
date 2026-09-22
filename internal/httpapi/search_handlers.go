package httpapi

import (
	"net/http"
	"sudoStream/internal/metadata"

	"github.com/gin-gonic/gin"
)

// SearchResponse is GET /api/search.
type SearchResponse struct {
	Items  []metadata.SearchHit `json:"items"`
	Total  int                  `json:"total"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

// Search indexed videos by filename and metadata.
//
//	@Summary		Search media
//	@Description	Finds indexed videos in readable libraries by filename and metadata tokens.
//	@Tags			media
//	@Produce		json
//	@Param			q		query		string	true	"Search query"
//	@Param			limit	query		int		false	"Page size (default 28, max 100)"
//	@Param			offset	query		int		false	"Offset (default 0)"
//	@Success		200		{object}	SearchResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/search [get]
func (h *handler) searchMedia(c *gin.Context) {
	if h.metadata == nil || h.access == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "search unavailable"})

		return
	}

	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	libraries, err := h.access.ListReadableLibraries(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errListLibrariesFailed})

		return
	}

	libraryIDs := make([]string, 0, len(libraries))
	for _, library := range libraries {
		libraryIDs = append(libraryIDs, library.ID)
	}

	opts := parsePageOpts(c)
	items, total, err := h.metadata.Search(
		c.Request.Context(),
		libraryIDs,
		c.Query("q"),
		opts.Limit,
		opts.Offset,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "search failed"})

		return
	}

	c.JSON(http.StatusOK, SearchResponse{
		Items:  items,
		Total:  total,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	})
}
